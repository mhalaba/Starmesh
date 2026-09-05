package hub

import (
	"os"
	"path/filepath"
)

func DefaultHome() string {
	if v := os.Getenv("STARMESH_HOME"); v != "" {
		return v
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ".starmesh"
	}
	return filepath.Join(h, ".starmesh")
}

func EnsureHome(home string) error {
	return os.MkdirAll(home, 0o700)
}

func IdentityPath(home string) string { return filepath.Join(home, "identity.json") }
func CachePath(home string) string    { return filepath.Join(home, "hub-cache.json") }
func QueuePath(home string) string    { return filepath.Join(home, "queue.json") }

func LoadOrCreateIdentity(home, name string) (*Identity, error) {
	if err := EnsureHome(home); err != nil {
		return nil, err
	}
	p := IdentityPath(home)
	if _, err := os.Stat(p); err == nil {
		id, err := LoadIdentity(p)
		if err != nil {
			return nil, err
		}
		if name != "" && id.Name == "" {
			id.Name = name
			_ = id.Save(p)
		}
		return id, nil
	}
	id, err := GenerateIdentity(name)
	if err != nil {
		return nil, err
	}
	if err := id.Save(p); err != nil {
		return nil, err
	}
	return id, nil
}
