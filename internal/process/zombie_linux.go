package process

import (
	"fmt"
	"os"
	"strings"
)

func isZombie(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	s := string(data)
	i := strings.LastIndex(s, ")")
	return i >= 0 && len(s) > i+2 && s[i+2] == 'Z'
}
