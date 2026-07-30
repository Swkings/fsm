package store

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/sftp"
	"github.com/swkings/fsm/config"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type sshJSONLStore struct {
	directory string
	filename  string
	location  string
	ssh       *ssh.Client
	client    *sftp.Client
	mu        sync.Mutex
}

// RemoteDirectoryEntry describes a directory or JSONL file available through SFTP.
type RemoteDirectoryEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
	Size      int64  `json:"size,omitempty"`
}

// RemoteDirectoryView is a browsable snapshot of one remote SFTP directory.
type RemoteDirectoryView struct {
	Path    string                 `json:"path"`
	Parent  string                 `json:"parent,omitempty"`
	Entries []RemoteDirectoryEntry `json:"entries"`
}

type sshSFTPConnection struct {
	address  string
	username string
	ssh      *ssh.Client
	client   *sftp.Client
}

func newSSHJSONL(cfg config.JSONL) (*sshJSONLStore, error) {
	remote := cfg.SSH
	if remote == nil {
		return nil, errors.New("jsonl ssh configuration is required")
	}
	connection, err := dialJSONLSSH(*remote)
	if err != nil {
		return nil, err
	}
	directory := path.Clean(strings.TrimSpace(cfg.Directory))
	if directory == "." || directory == "" {
		_ = connection.close()
		return nil, errors.New("jsonl ssh directory is required")
	}
	if err = connection.client.MkdirAll(directory); err != nil {
		_ = connection.close()
		return nil, fmt.Errorf("prepare jsonl ssh directory %s: %w", directory, err)
	}
	location := fmt.Sprintf("ssh://%s@%s/%s", connection.username, connection.address, strings.TrimPrefix(directory, "/"))
	return &sshJSONLStore{directory: directory, filename: cfg.Filename, location: location, ssh: connection.ssh, client: connection.client}, nil
}

func dialJSONLSSH(remote config.SSH) (*sshSFTPConnection, error) {
	address := strings.TrimSpace(remote.Address)
	if address == "" {
		return nil, errors.New("jsonl ssh address is required")
	}
	if _, _, err := net.SplitHostPort(address); err != nil {
		address = net.JoinHostPort(strings.Trim(address, "[]"), "22")
	}
	username := strings.TrimSpace(remote.Username)
	if username == "" {
		if current, err := user.Current(); err == nil {
			username = current.Username
		}
	}
	if username == "" {
		return nil, errors.New("jsonl ssh username is required")
	}
	auth, err := sshAuthMethods(remote)
	if err != nil {
		return nil, err
	}
	if len(auth) == 0 {
		return nil, errors.New("jsonl ssh password or private key file is required")
	}
	hostKey, err := sshHostKeyCallback(remote)
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(remote.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	connection, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, fmt.Errorf("connect jsonl ssh %s: %w", address, err)
	}
	clientConnection, channels, requests, err := ssh.NewClientConn(connection, address, &ssh.ClientConfig{User: username, Auth: auth, HostKeyCallback: hostKey, Timeout: timeout})
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("authenticate jsonl ssh %s: %w", address, err)
	}
	sshClient := ssh.NewClient(clientConnection, channels, requests)
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		_ = sshClient.Close()
		return nil, fmt.Errorf("start jsonl sftp %s: %w", address, err)
	}
	return &sshSFTPConnection{address: address, username: username, ssh: sshClient, client: sftpClient}, nil
}

func (c *sshSFTPConnection) close() error {
	err := c.client.Close()
	if sshErr := c.ssh.Close(); err == nil {
		err = sshErr
	}
	return err
}

// BrowseSSHJSONL connects to an SSH server and lists its child directories and
// JSONL files without registering a replay source or creating remote paths.
func BrowseSSHJSONL(ctx context.Context, cfg config.JSONL) (RemoteDirectoryView, error) {
	if err := ctx.Err(); err != nil {
		return RemoteDirectoryView{}, err
	}
	if cfg.SSH == nil {
		return RemoteDirectoryView{}, errors.New("jsonl ssh configuration is required")
	}
	connection, err := dialJSONLSSH(*cfg.SSH)
	if err != nil {
		return RemoteDirectoryView{}, err
	}
	defer connection.close()

	directory := strings.TrimSpace(cfg.Directory)
	if directory == "" {
		directory = "."
	}
	directory, err = connection.client.RealPath(directory)
	if err != nil {
		return RemoteDirectoryView{}, fmt.Errorf("resolve jsonl ssh directory %s: %w", cfg.Directory, err)
	}
	entries, err := connection.client.ReadDir(directory)
	if err != nil {
		return RemoteDirectoryView{}, fmt.Errorf("browse jsonl ssh directory %s: %w", directory, err)
	}
	view := RemoteDirectoryView{Path: directory, Entries: make([]RemoteDirectoryEntry, 0, len(entries))}
	if parent := path.Dir(directory); parent != directory {
		view.Parent = parent
	}
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return RemoteDirectoryView{}, err
		}
		if !entry.IsDir() && (entry.Mode()&os.ModeType != 0 || !strings.EqualFold(path.Ext(entry.Name()), ".jsonl")) {
			continue
		}
		view.Entries = append(view.Entries, RemoteDirectoryEntry{
			Name: entry.Name(), Path: path.Join(directory, entry.Name()), Directory: entry.IsDir(), Size: entry.Size(),
		})
	}
	sort.Slice(view.Entries, func(i, j int) bool {
		if view.Entries[i].Directory != view.Entries[j].Directory {
			return view.Entries[i].Directory
		}
		return strings.ToLower(view.Entries[i].Name) < strings.ToLower(view.Entries[j].Name)
	})
	return view, nil
}

func sshAuthMethods(cfg config.SSH) ([]ssh.AuthMethod, error) {
	methods := make([]ssh.AuthMethod, 0, 2)
	if cfg.Password != "" {
		methods = append(methods, ssh.Password(cfg.Password))
	}
	if strings.TrimSpace(cfg.PrivateKeyFile) == "" {
		return methods, nil
	}
	key, err := os.ReadFile(expandHome(cfg.PrivateKeyFile))
	if err != nil {
		return nil, fmt.Errorf("read jsonl ssh private key: %w", err)
	}
	var signer ssh.Signer
	if cfg.PrivateKeyPassphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(cfg.PrivateKeyPassphrase))
	} else {
		signer, err = ssh.ParsePrivateKey(key)
	}
	if err != nil {
		return nil, fmt.Errorf("parse jsonl ssh private key: %w", err)
	}
	return append(methods, ssh.PublicKeys(signer)), nil
}

func sshHostKeyCallback(cfg config.SSH) (ssh.HostKeyCallback, error) {
	if cfg.InsecureIgnoreHostKey {
		return ssh.InsecureIgnoreHostKey(), nil //nolint:gosec // Explicit opt-in for trusted development networks.
	}
	knownHostsFile := strings.TrimSpace(cfg.KnownHostsFile)
	if knownHostsFile == "" {
		knownHostsFile = "~/.ssh/known_hosts"
	}
	callback, err := knownhosts.New(expandHome(knownHostsFile))
	if err != nil {
		return nil, fmt.Errorf("load jsonl ssh known hosts: %w", err)
	}
	return callback, nil
}

func expandHome(value string) string {
	value = strings.TrimSpace(value)
	if value == "~" || strings.HasPrefix(value, "~/") {
		if current, err := user.Current(); err == nil {
			return filepath.Join(current.HomeDir, strings.TrimPrefix(value, "~/"))
		}
	}
	return value
}

func (s *sshJSONLStore) Descriptor() Descriptor {
	return Descriptor{Driver: config.StoreDriverJSONL, Location: s.location}
}

func (s *sshJSONLStore) Save(ctx context.Context, record Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	file, err := s.client.OpenFile(path.Join(s.directory, s.filename), os.O_CREATE|os.O_APPEND|os.O_WRONLY)
	if err != nil {
		return err
	}
	_, err = file.Write(append(data, '\n'))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func (s *sshJSONLStore) read(ctx context.Context) ([]Record, error) {
	entries, err := s.client.ReadDir(s.directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []Record
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() || path.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		file, openErr := s.client.Open(path.Join(s.directory, entry.Name()))
		if openErr != nil {
			return nil, openErr
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 65536), 32<<20)
		for scanner.Scan() {
			var record Record
			if err = json.Unmarshal(scanner.Bytes(), &record); err != nil {
				_ = file.Close()
				return nil, err
			}
			records = append(records, record)
		}
		err = scanner.Err()
		_ = file.Close()
		if err != nil {
			return nil, err
		}
	}
	return records, nil
}

func (s *sshJSONLStore) List(ctx context.Context, query Query) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.read(ctx)
	if err != nil {
		return nil, err
	}
	return filter(records, query), ctx.Err()
}

func (s *sshJSONLStore) Latest(ctx context.Context, key string) (Record, error) {
	return latest(ctx, s, key)
}

func (s *sshJSONLStore) Get(ctx context.Context, id string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.read(ctx)
	if err != nil {
		return Record{}, err
	}
	for _, record := range records {
		if record.ID == id {
			return record, nil
		}
	}
	return Record{}, ErrNotFound
}

func (s *sshJSONLStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.client.ReadDir(s.directory)
	if err != nil {
		return err
	}
	found := false
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || path.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		recordPath := path.Join(s.directory, entry.Name())
		file, openErr := s.client.Open(recordPath)
		if openErr != nil {
			return openErr
		}
		var kept [][]byte
		changed := false
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 65536), 32<<20)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			var record Record
			if err = json.Unmarshal(line, &record); err != nil {
				_ = file.Close()
				return err
			}
			if record.ID == id {
				changed, found = true, true
				continue
			}
			kept = append(kept, line)
		}
		err = scanner.Err()
		_ = file.Close()
		if err != nil || !changed {
			if err != nil {
				return err
			}
			continue
		}
		tempPath := path.Join(s.directory, ".fsm-"+uuid.NewString()+".jsonl")
		temp, createErr := s.client.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
		if createErr != nil {
			return createErr
		}
		for _, line := range kept {
			if _, err = temp.Write(append(line, '\n')); err != nil {
				break
			}
		}
		if closeErr := temp.Close(); err == nil {
			err = closeErr
		}
		if err == nil {
			err = s.client.PosixRename(tempPath, recordPath)
		}
		if err != nil {
			_ = s.client.Remove(tempPath)
			return err
		}
	}
	if !found {
		return ErrNotFound
	}
	return nil
}

func (s *sshJSONLStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.client.Close()
	if sshErr := s.ssh.Close(); err == nil {
		err = sshErr
	}
	return err
}
