// Package source exposes read-only access to the Target App source tree.
package source

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tchoupi663/weave/services/agent/internal/scope"
)

type Config struct {
	SourceRoots []string
	Guard       *scope.Guard
}

var ErrPathNotAllowed = errors.New("path_not_allowed: request outside the allowed source roots")

type Client struct {
	roots []string
	guard *scope.Guard
}

func New(cfg Config) *Client {
	roots := make([]string, 0, len(cfg.SourceRoots))
	for _, root := range cfg.SourceRoots {
		abs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		roots = append(roots, abs)
	}
	return &Client{roots: roots, guard: cfg.Guard}
}

func Register(server *mcp.Server, cfg Config) {
	client := New(cfg)
	mcp.AddTool(server, &mcp.Tool{Name: "list_directory", Description: "List a directory inside the Target App source roots."}, client.listDirectory)
	mcp.AddTool(server, &mcp.Tool{Name: "read_file", Description: "Read a text file inside the Target App source roots."}, client.readFile)
	mcp.AddTool(server, &mcp.Tool{Name: "search_source", Description: "Search the Target App source roots for a substring."}, client.searchSource)
}

type listIn struct {
	Path string `json:"path,omitempty" jsonschema:"directory path inside an allowed source root; empty lists the roots"`
}

type Entry struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size,omitempty"`
}

type listOut struct {
	Path    string  `json:"path"`
	Entries []Entry `json:"entries"`
}

func (c *Client) listDirectory(ctx context.Context, _ *mcp.CallToolRequest, in listIn) (*mcp.CallToolResult, listOut, error) {
	if strings.TrimSpace(in.Path) == "" {
		entries := make([]Entry, 0, len(c.roots))
		for _, root := range c.roots {
			entries = append(entries, Entry{Name: c.display(root), Type: "root"})
		}
		return nil, listOut{Path: ".", Entries: entries}, nil
	}
	abs, err := c.allowedPath(in.Path)
	if err != nil {
		return nil, listOut{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, listOut{}, err
	}
	if !info.IsDir() {
		return nil, listOut{}, fmt.Errorf("%q is not a directory", in.Path)
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, listOut{}, err
	}
	result := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		item := Entry{Name: entry.Name(), Type: "file"}
		if entry.IsDir() {
			item.Type = "directory"
		} else if info, err := entry.Info(); err == nil {
			item.Size = info.Size()
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Type != result[j].Type {
			return result[i].Type < result[j].Type
		}
		return result[i].Name < result[j].Name
	})
	return nil, listOut{Path: c.display(abs), Entries: result}, nil
}

type readIn struct {
	Path     string `json:"path" jsonschema:"file path inside an allowed source root"`
	MaxBytes int    `json:"max_bytes,omitempty" jsonschema:"maximum bytes to return; defaults to 200000"`
}

type readOut struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

func (c *Client) readFile(ctx context.Context, _ *mcp.CallToolRequest, in readIn) (*mcp.CallToolResult, readOut, error) {
	abs, err := c.allowedPath(in.Path)
	if err != nil {
		return nil, readOut{}, err
	}
	maxBytes := in.MaxBytes
	if maxBytes <= 0 || maxBytes > 1_000_000 {
		maxBytes = 200_000
	}
	file, err := os.Open(abs)
	if err != nil {
		return nil, readOut{}, err
	}
	defer file.Close()
	buffer := make([]byte, maxBytes)
	read, err := file.Read(buffer)
	if err != nil && read == 0 {
		return nil, readOut{}, err
	}
	content := string(buffer[:read])
	truncated := read == maxBytes
	return nil, readOut{Path: c.display(abs), Content: content, Truncated: truncated}, nil
}

type searchIn struct {
	Query      string `json:"query" jsonschema:"case-sensitive substring to search for"`
	Path       string `json:"path,omitempty" jsonschema:"optional file or directory inside an allowed source root"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"maximum matches; defaults to 50"`
}

type Match struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type searchOut struct {
	Query     string  `json:"query"`
	Matches   []Match `json:"matches"`
	Truncated bool    `json:"truncated"`
}

func (c *Client) searchSource(ctx context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, searchOut, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, searchOut{}, fmt.Errorf("query is required")
	}
	maxResults := in.MaxResults
	if maxResults <= 0 || maxResults > 200 {
		maxResults = 50
	}
	roots := c.roots
	if strings.TrimSpace(in.Path) != "" {
		abs, err := c.allowedPath(in.Path)
		if err != nil {
			return nil, searchOut{}, err
		}
		roots = []string{abs}
	}
	out := searchOut{Query: in.Query, Matches: make([]Match, 0)}
	for _, root := range roots {
		if err := c.walkMatches(ctx, root, in.Query, maxResults, &out); err != nil {
			return nil, searchOut{}, err
		}
		if len(out.Matches) >= maxResults {
			out.Truncated = true
			break
		}
	}
	sort.Slice(out.Matches, func(i, j int) bool {
		if out.Matches[i].Path != out.Matches[j].Path {
			return out.Matches[i].Path < out.Matches[j].Path
		}
		return out.Matches[i].Line < out.Matches[j].Line
	})
	return nil, out, nil
}

func (c *Client) walkMatches(ctx context.Context, root, query string, maxResults int, out *searchOut) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return c.matchFile(root, query, maxResults, out)
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if len(out.Matches) >= maxResults {
			return nil
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", ".cache", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		return c.matchFile(path, query, maxResults, out)
	})
}

func (c *Client) matchFile(path, query string, maxResults int, out *searchOut) error {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > 2_000_000 {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 512*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if !strings.Contains(line, query) {
			continue
		}
		out.Matches = append(out.Matches, Match{Path: c.display(path), Line: lineNumber, Text: truncate(line, 500)})
		if len(out.Matches) >= maxResults {
			break
		}
	}
	return scanner.Err()
}

func (c *Client) allowedPath(raw string) (string, error) {
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	state, err := c.guard.Load()
	if err != nil {
		return "", err
	}
	if state != nil {
		if !state.SourcePathAllowed(abs) {
			return "", scope.ErrDenied
		}
		return abs, nil
	}
	for _, root := range c.roots {
		if abs == root || strings.HasPrefix(abs, root+string(filepath.Separator)) {
			return abs, nil
		}
	}
	return "", ErrPathNotAllowed
}

func (c *Client) display(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	working, err := os.Getwd()
	if err == nil {
		if relative, err := filepath.Rel(working, absolute); err == nil && !strings.HasPrefix(relative, "..") {
			return relative
		}
	}
	return filepath.Base(absolute)
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + " [truncated]"
}
