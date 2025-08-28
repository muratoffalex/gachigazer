package markdown

import (
	"fmt"
	"os/exec"
)

func checkPython() (string, bool) {
	for _, cmd := range []string{"python3", "python"} {
		path, err := exec.LookPath(cmd)
		if err == nil {
			return path, true
		}
	}
	return "", false
}

func checkPythonPackage(pythonPath string, packageName string) bool {
	cmd := exec.Command(pythonPath, "-c", fmt.Sprintf("import %s", packageName))
	return cmd.Run() == nil
}
