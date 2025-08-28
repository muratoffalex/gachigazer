package service

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"time"
)

func ConvertOggToMP3(oggData []byte) ([]byte, error) {
	inputFile := fmt.Sprintf("/tmp/input_%d.ogg", time.Now().UnixNano())
	outputFile := fmt.Sprintf("/tmp/output_%d.mp3", time.Now().UnixNano())

	defer os.Remove(inputFile)
	defer os.Remove(outputFile)

	if err := os.WriteFile(inputFile, oggData, 0o644); err != nil {
		return nil, err
	}

	cmd := exec.Command("ffmpeg",
		"-i", inputFile,
		"-acodec", "libmp3lame",
		"-q:a", "4", // Quality 2-8 (lower = better)
		outputFile,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg error: %v\n%s", err, stderr.String())
	}

	return os.ReadFile(outputFile)
}
