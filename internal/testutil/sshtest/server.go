package sshtest

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Env struct {
	Host           string
	Addr           string
	KeyPath        string
	KnownHostsPath string
	Port           int
}

type server struct {
	listener net.Listener
	mu       sync.Mutex
	conns    map[net.Conn]struct{}
	stopping bool
	wg       sync.WaitGroup
}

func Start(t *testing.T) *Env {
	t.Helper()
	_, hostKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		t.Fatalf("host signer: %v", err)
	}
	_, clientKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	clientSigner, err := ssh.NewSignerFromKey(clientKey)
	if err != nil {
		t.Fatalf("client signer: %v", err)
	}

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_ed25519")
	block, err := ssh.MarshalPrivateKey(clientKey, "")
	if err != nil {
		t.Fatalf("marshal client key: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write client key: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

	knownHostsPath := filepath.Join(dir, "known_hosts")
	line := knownhosts.Line([]string{addr}, hostSigner.PublicKey()) + "\n"
	if err := os.WriteFile(knownHostsPath, []byte(line), 0o600); err != nil {
		listener.Close()
		t.Fatalf("write known hosts: %v", err)
	}

	config := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if bytes.Equal(key.Marshal(), clientSigner.PublicKey().Marshal()) {
				return nil, nil
			}
			return nil, errors.New("unauthorized key")
		},
	}
	config.AddHostKey(hostSigner)

	srv := &server{listener: listener, conns: make(map[net.Conn]struct{})}
	srv.wg.Add(1)
	go srv.accept(config)
	t.Cleanup(srv.stop)

	return &Env{
		Host:           "127.0.0.1",
		Addr:           addr,
		KeyPath:        keyPath,
		KnownHostsPath: knownHostsPath,
		Port:           port,
	}
}

func (s *server) stop() {
	s.mu.Lock()
	s.stopping = true
	s.listener.Close()
	for conn := range s.conns {
		conn.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *server) accept(config *ssh.ServerConfig) {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		if s.stopping {
			s.mu.Unlock()
			conn.Close()
			continue
		}
		s.conns[conn] = struct{}{}
		s.wg.Add(1)
		s.mu.Unlock()
		go s.serve(conn, config)
	}
}

func (s *server) serve(conn net.Conn, config *ssh.ServerConfig) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
		conn.Close()
	}()
	serverConn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer serverConn.Close()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ssh.DiscardRequests(reqs)
	}()
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		s.wg.Add(1)
		go s.handleSession(channel, requests)
	}
}

func (s *server) handleSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer s.wg.Done()
	defer channel.Close()
	for request := range requests {
		if request.Type != "exec" {
			request.Reply(false, nil)
			continue
		}
		var payload struct {
			Command string
		}
		if err := ssh.Unmarshal(request.Payload, &payload); err != nil {
			request.Reply(false, nil)
			return
		}
		request.Reply(true, nil)
		status := runCommand(channel, payload.Command, requests)
		exit := struct {
			Status uint32
		}{Status: uint32(status)}
		channel.SendRequest("exit-status", false, ssh.Marshal(&exit))
		return
	}
}

func runCommand(channel ssh.Channel, command string, requests <-chan *ssh.Request) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stdout = channel
	cmd.Stderr = channel.Stderr()
	if err := cmd.Start(); err != nil {
		return 127
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	for {
		select {
		case err := <-done:
			return exitCode(err)
		case request, ok := <-requests:
			if !ok {
				cancel()
				return exitCode(<-done)
			}
			if request.Type == "signal" {
				cancel()
				return exitCode(<-done)
			}
			request.Reply(false, nil)
		}
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return code
		}
	}
	return 1
}
