package lock

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Lockfile struct {
	Version int               `json:"version"`
	Images  map[string]string `json:"images"`
}

func New() *Lockfile {
	return &Lockfile{Version: 1, Images: map[string]string{}}
}

func Load(path string) (*Lockfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lf Lockfile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, err
	}

	if lf.Images == nil {
		lf.Images = map[string]string{}
	}

	return &lf, nil
}

func Save(path string, lf *Lockfile) error {
	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func Apply(image string, lf *Lockfile) string {
	if lf == nil || image == "" {
		return image
	}
	if strings.Contains(image, "@sha256:") {
		return image
	}

	digest, ok := lf.Images[image]
	if !ok || digest == "" {
		return image
	}

	base := stripTag(image)
	return fmt.Sprintf("%s@%s", base, digest)
}

func stripTag(image string) string {
	if strings.Contains(image, "@") {
		return strings.Split(image, "@")[0]
	}

	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	if lastColon > lastSlash {
		return image[:lastColon]
	}

	return image
}
