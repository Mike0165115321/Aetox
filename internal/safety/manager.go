package safety

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"aetox-cli/internal/config"
	"aetox-cli/internal/contracts"
)

type Manager struct {
	cfg    config.Config
	in     io.Reader
	out    io.Writer
}

func NewManager(cfg config.Config) *Manager {
	return &Manager{
		cfg: cfg,
		in:  os.Stdin,
		out: os.Stdout,
	}
}

func (m *Manager) Approve(ctx context.Context, step contracts.TaskStep) (bool, error) {
	if step.RiskLevel == contracts.RiskLow || m.cfg.AutoApprove {
		return true, nil
	}

	prompt := buildApprovalPrompt(step, m.cfg)
	if _, err := fmt.Fprintln(m.out, prompt); err != nil {
		return false, err
	}

	answerCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(m.in)
		text, err := reader.ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		answerCh <- text
	}()

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case err := <-errCh:
		return false, err
	case ans := <-answerCh:
		text := strings.ToLower(strings.TrimSpace(ans))
		return text == "y" || text == "yes", nil
	case <-time.After(time.Duration(m.cfg.ApprovalTimeoutSec) * time.Second):
		return false, fmt.Errorf("approval timed out")
	}
}

func buildApprovalPrompt(step contracts.TaskStep, cfg config.Config) string {
	target := describeTarget(step)
	return fmt.Sprintf("Risk %q for step %d (%s). Target: %s. Approve? [y/N] (auto timeout %ds)",
		step.RiskLevel, step.ID, step.Description, target, cfg.ApprovalTimeoutSec)
}

func describeTarget(step contracts.TaskStep) string {
	parts := []string{}
	for _, key := range []string{"path", "source", "target", "pattern"} {
		if value, ok := step.Params[key]; ok {
			if txt, ok := value.(string); ok && strings.TrimSpace(txt) != "" {
				parts = append(parts, fmt.Sprintf("%s=%s", key, txt))
			}
		}
	}
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, ", ")
}
