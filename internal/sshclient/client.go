package sshclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

var (
	ErrNotExist = errors.New("remote file does not exist")
	ErrVerify   = errors.New("remote read-back mismatch")
)

const (
	defaultDialTimeout = 10 * time.Second
	fileTimeout        = 60 * time.Second
	keepaliveInterval  = 15 * time.Second
	keepaliveRequest   = "keepalive@openssh.com"
	notExistExitCode   = 42
)

type Options struct {
	Host       string
	Port       int
	User       string
	KeyPath    string
	KnownHosts string
	Passphrase func() (string, error)
	Timeout    time.Duration
}

type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

type Client struct {
	conn      *ssh.Client
	agentConn net.Conn
	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
}

func Dial(ctx context.Context, opts Options) (*Client, error) {
	auth, agentConn, err := authMethods(opts)
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(opts.Host, strconv.Itoa(opts.Port))
	hostKey, err := hostKeyCallback(opts)
	if err != nil {
		closeConn(agentConn)
		return nil, fmt.Errorf("known hosts %s: %w", opts.KnownHosts, err)
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultDialTimeout
	}
	config := &ssh.ClientConfig{
		User:            opts.User,
		Auth:            auth,
		HostKeyCallback: hostKey,
	}
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		closeConn(agentConn)
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		conn.Close()
		closeConn(agentConn)
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		closeConn(agentConn)
		return nil, fmt.Errorf("ssh %s: %w", addr, err)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		sshConn.Close()
		closeConn(agentConn)
		return nil, fmt.Errorf("ssh %s: %w", addr, err)
	}
	client := &Client{
		conn:      ssh.NewClient(sshConn, chans, reqs),
		agentConn: agentConn,
		done:      make(chan struct{}),
	}
	go client.keepalive()
	return client, nil
}

func authMethods(opts Options) ([]ssh.AuthMethod, net.Conn, error) {
	var methods []ssh.AuthMethod
	if opts.KeyPath != "" {
		key, err := loadKey(opts)
		if err != nil {
			return nil, nil, err
		}
		methods = append(methods, ssh.PublicKeys(key))
	}
	var agentConn net.Conn
	var agentErr error
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		conn, err := net.Dial("unix", sock)
		if err != nil {
			agentErr = err
		} else {
			agentConn = conn
			methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
		}
	}
	if len(methods) == 0 {
		if agentErr != nil {
			return nil, agentConn, fmt.Errorf("no ssh authentication method configured: ssh agent: %w", agentErr)
		}
		return nil, agentConn, errors.New("no ssh authentication method configured")
	}
	return methods, agentConn, nil
}

func hostKeyCallback(opts Options) (ssh.HostKeyCallback, error) {
	check, err := knownhosts.New(opts.KnownHosts)
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(opts.Host, strconv.Itoa(opts.Port))
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := check(hostname, remote, key)
		if err == nil {
			return nil
		}
		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err
		}
		presented := ssh.FingerprintSHA256(key)
		if len(keyErr.Want) == 0 {
			return fmt.Errorf("host key for %s is not in %s: presented %s; add it with: %s", addr, opts.KnownHosts, presented, keyscanHint(opts))
		}
		accepted := make([]string, 0, len(keyErr.Want))
		for _, want := range keyErr.Want {
			accepted = append(accepted, ssh.FingerprintSHA256(want.Key))
		}
		return fmt.Errorf("host key for %s changed: presented %s, accepted %s", addr, presented, strings.Join(accepted, ", "))
	}, nil
}

func keyscanHint(opts Options) string {
	hint := "ssh-keyscan -H"
	if opts.Port != 22 {
		hint += " -p " + strconv.Itoa(opts.Port)
	}
	return hint + " " + shellQuote(opts.Host) + " >> " + shellQuote(opts.KnownHosts)
}

func loadKey(opts Options) (ssh.Signer, error) {
	data, err := os.ReadFile(opts.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("ssh key %s: %w", opts.KeyPath, err)
	}
	key, err := ssh.ParsePrivateKey(data)
	var missing *ssh.PassphraseMissingError
	if errors.As(err, &missing) {
		if opts.Passphrase == nil {
			return nil, fmt.Errorf("ssh key %s: encrypted key and no passphrase configured", opts.KeyPath)
		}
		passphrase, passErr := opts.Passphrase()
		if passErr != nil {
			return nil, fmt.Errorf("ssh key %s: passphrase: %w", opts.KeyPath, passErr)
		}
		key, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(passphrase))
	}
	if err != nil {
		return nil, fmt.Errorf("ssh key %s: %w", opts.KeyPath, err)
	}
	return key, nil
}

func closeConn(conn net.Conn) {
	if conn != nil {
		conn.Close()
	}
}

func (c *Client) keepalive() {
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			if _, _, err := c.conn.SendRequest(keepaliveRequest, true, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
		closeConn(c.agentConn)
		c.closeErr = c.conn.Close()
	})
	return c.closeErr
}

func (c *Client) ReadFile(ctx context.Context, path string) ([]byte, error) {
	ctx, cancel := withFileTimeout(ctx)
	defer cancel()
	quoted := shellQuote(path)
	result, err := c.Run(ctx, "if [ -f "+quoted+" ]; then cat "+quoted+"; else exit "+strconv.Itoa(notExistExitCode)+"; fi", 0)
	if err != nil {
		return nil, err
	}
	switch {
	case result.ExitCode == notExistExitCode:
		return nil, fmt.Errorf("%s: %w", path, ErrNotExist)
	case result.ExitCode != 0:
		return nil, fmt.Errorf("read %s: exit %d: %s", path, result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return []byte(result.Stdout), nil
}

func (c *Client) Run(ctx context.Context, command string, timeout time.Duration) (CommandResult, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	session, err := c.conn.NewSession()
	if err != nil {
		return CommandResult{}, fmt.Errorf("open session: %w", err)
	}
	defer session.Close()
	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if err := session.Start(command); err != nil {
		return CommandResult{}, fmt.Errorf("start command: %w", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- session.Wait()
	}()
	select {
	case waitErr := <-done:
		result := CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}
		if waitErr == nil {
			return result, nil
		}
		var exitErr *ssh.ExitError
		if errors.As(waitErr, &exitErr) {
			result.ExitCode = exitErr.ExitStatus()
			return result, nil
		}
		return result, fmt.Errorf("wait for command: %w", waitErr)
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		session.Close()
		return CommandResult{}, ctx.Err()
	}
}

func withFileTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, fileTimeout)
}
