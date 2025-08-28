package markdown

import (
	"bytes"
	_ "embed"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/muratoffalex/gachigazer/internal/logger"
)

type MarkdownProcessor struct {
	binaryPath string
}

func NewMarkdownProcessor(binaryPath string, logger logger.Logger) *MarkdownProcessor {
	// TODO: move this checks to healthcheck
	if pythonPath, ok := checkPython(); ok {
		logger.WithField("path", pythonPath).Info("Python detected in system")

		checkDeps := []string{"telegramify_markdown"}
		missing := make([]string, 0)

		for _, pkg := range checkDeps {
			if !checkPythonPackage(pythonPath, pkg) {
				missing = append(missing, pkg)
			}
		}

		if len(missing) > 0 {
			logger.Warn("Missing Python packages",
				" python ", pythonPath,
				" missing ", strings.Join(missing, ", "),
				" hint ", "Run: pip install "+strings.Join(missing, " "))
		}
	} else {
		logger.Warn("Python not found in system path")
	}

	return &MarkdownProcessor{
		binaryPath: binaryPath,
	}
}

//go:embed scripts/telegramify
var telegramifyScript string

func (c *MarkdownProcessor) Convert(text string) (string, error) {
	var binaryPath string
	var err error
	if c.binaryPath != "" {
		binaryPath = c.binaryPath
	} else {
		binaryPath, err = getEmbededScript()
		if err != nil {
			return "", err
		}
		defer os.Remove(binaryPath)
	}

	cmd := exec.Command(binaryPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return "", err
	}

	if _, err := io.WriteString(stdin, text); err != nil {
		stdin.Close()
		return "", err
	}
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		return "", err
	}

	return stdout.String(), nil
}

func Escape(text string) string {
	text = strings.ToValidUTF8(text, "")

	specialChars := []string{
		"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!",
	}

	for _, char := range specialChars {
		text = strings.ReplaceAll(text, char, "\\"+char)
	}

	return text
}

func getEmbededScript() (string, error) {
	tmpFile, err := os.CreateTemp("", "telegramify-*")
	if err != nil {
		return "", err
	}

	if _, err := tmpFile.WriteString(telegramifyScript); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", err
	}

	if err := tmpFile.Chmod(0o755); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", err
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}
