// Command loffice-mcp serves the documents of a directory to an AI model as
// an MCP server over standard input and output, for a local client such as
// Claude Code to start. It is for development: a host serving real users
// mounts the handler of package mcp beside its hub, which asks it who may do
// what.
//
//	loffice-mcp -root ~/documents -name Assistant
//
// Documents are the files under the root, named by their path from it; no
// path leads out of the root, not through "..", an absolute path or a link.
// Edits are saved as the hub of the host would: after a pause, and when the
// client closes the input.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/citadellefr/loffice"
	"github.com/citadellefr/loffice/mcp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("loffice-mcp", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dir := flags.String("root", "", "directory whose documents are served (required)")
	name := flags.String("name", "Assistant", "name the AI signs its revisions and comments with")
	timeout := flags.Duration("timeout", 2*time.Minute, "longest a call may take")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *dir == "" || flags.NArg() > 0 {
		fmt.Fprintln(stderr, "usage: loffice-mcp -root directory [-name name] [-timeout duration]")
		return 2
	}
	root, err := os.OpenRoot(*dir)
	if err != nil {
		fmt.Fprintln(stderr, "loffice-mcp:", err)
		return 1
	}
	defer root.Close()
	store := dirStore{root}
	hub := loffice.NewHub(store, loffice.Options{}, loffice.Limits{})
	srv, err := mcp.New(mcp.Config{
		Hub:       hub,
		Name:      *name,
		Timeout:   *timeout,
		Authorize: func(_ context.Context, a mcp.Access) error { return local(a.Doc) },
		List:      func(context.Context, mcp.Access) ([]mcp.Document, error) { return store.list() },
	})
	if err != nil {
		fmt.Fprintln(stderr, "loffice-mcp:", err)
		return 1
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ctx, stdin, stdout) }()
	select {
	case err = <-served:
	case <-ctx.Done():
	}
	srv.Close()
	flush, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if cerr := hub.Close(flush); cerr != nil {
		err = errors.Join(err, cerr)
	}
	if err != nil {
		fmt.Fprintln(stderr, "loffice-mcp:", err)
		return 1
	}
	return 0
}

// ErrOutside is a path that does not stay under the root.
var ErrOutside = errors.New("the path does not stay under the root")

// local checks that the key of a document is a path under the root: relative,
// clean, without "..".
func local(key string) error {
	if key != "" && (!fs.ValidPath(key) || key == "." || strings.ContainsAny(key, `\`+"\x00")) {
		return fmt.Errorf("%w: %q", ErrOutside, key)
	}
	return nil
}

// dirStore keeps the documents in a directory. The root refuses what leaves
// it, links included.
type dirStore struct{ root *os.Root }

func (s dirStore) Load(_ context.Context, key string) ([]byte, error) {
	if err := local(key); err != nil || key == "" {
		return nil, errors.Join(err, fs.ErrNotExist)
	}
	return s.root.ReadFile(key)
}

func (s dirStore) Save(_ context.Context, key string, data []byte) error {
	if err := local(key); err != nil || key == "" {
		return errors.Join(err, fs.ErrNotExist)
	}
	info, err := s.root.Stat(key)
	if err != nil {
		return err
	}
	tmp := path.Join(path.Dir(key), fmt.Sprintf(".%s.%d.tmp", path.Base(key), time.Now().UnixNano()))
	if err := s.root.WriteFile(tmp, data, info.Mode().Perm()); err != nil {
		_ = s.root.Remove(tmp)
		return err
	}
	if err := s.root.Rename(tmp, key); err != nil {
		_ = s.root.Remove(tmp)
		return err
	}
	return nil
}

// extensions are those of the documents a hub serves.
var extensions = []string{".docx", ".docm", ".dotx", ".dotm", ".pptx", ".pptm", ".ppsx", ".potx", ".potm", ".xlsx", ".xlsm", ".xltx", ".xltm", ".csv", ".txt"}

// list is the documents under the root.
func (s dirStore) list() ([]mcp.Document, error) {
	docs := []mcp.Document{}
	err := fs.WalkDir(s.root.FS(), ".", func(name string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return nil
		case d.IsDir() && name != "." && strings.HasPrefix(d.Name(), "."):
			return fs.SkipDir
		case d.Type().IsRegular() && !strings.HasPrefix(d.Name(), ".") && slices.Contains(extensions, strings.ToLower(path.Ext(name))):
			docs = append(docs, mcp.Document{Doc: name, Title: strings.TrimSuffix(d.Name(), path.Ext(name))})
		}
		return nil
	})
	return docs, err
}
