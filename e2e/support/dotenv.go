package support

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

func LoadEnvironment() error {
	repoRoot, err := repoRoot()
	if err != nil {
		return err
	}

	envPath := filepath.Join(repoRoot, ".env")
	if _, err := os.Stat(envPath); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return godotenv.Load(envPath)
}
