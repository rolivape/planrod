package sessions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const SummaryThreshold = 2500

type Manager struct {
	Dir string // /opt/plan/sessions
}

func (m *Manager) Init() error {
	if err := os.MkdirAll(m.Dir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(m.Dir, "archived"), 0755); err != nil {
		return err
	}

	gitDir := filepath.Join(m.Dir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return nil
	}

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "planrod@hiverod.local"},
		{"git", "config", "user.name", "PlanRod"},
	}
	for _, c := range cmds {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = m.Dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("sessions git init %v: %s: %w", c, out, err)
		}
	}

	if err := os.WriteFile(filepath.Join(m.Dir, ".gitignore"), []byte(""), 0644); err != nil {
		return err
	}
	return m.gitCommit("initial commit")
}

func (m *Manager) IsValid() bool {
	_, err := os.Stat(filepath.Join(m.Dir, ".git"))
	return err == nil
}

// WriteSummary writes a long summary to a file and commits it.
// Returns the relative path to the file.
func (m *Manager) WriteSummary(id int64, content string) (string, error) {
	filename := fmt.Sprintf("%d.md", id)
	path := filepath.Join(m.Dir, filename)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}

	if err := m.gitCommit(fmt.Sprintf("session %d: log summary", id)); err != nil {
		return "", err
	}

	return filename, nil
}

// ReadSummary reads a session summary from a file.
func (m *Manager) ReadSummary(path string) (string, error) {
	fullPath := filepath.Join(m.Dir, path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("session file %q not found", path)
	}
	return string(data), nil
}

func (m *Manager) gitCommit(message string) error {
	cmds := [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-m", message, "--author", "PlanRod <planrod@hiverod.local>", "--allow-empty"},
	}
	for _, c := range cmds {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = m.Dir
		cmd.Run()
	}
	return nil
}
