package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// checkForUpdate checks GitHub for a newer version and prints a message if available.
func checkForUpdate() {
	if version == "dev" {
		return
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/AnassKartit/tfoutdated/releases/latest")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(version, "v")

	if latest != "" && latest != current {
		fmt.Printf("\nA new version of tfoutdated is available: %s → %s\n", current, latest)
		fmt.Println("Upgrade: go install github.com/anasskartit/tfoutdated@latest")
		fmt.Println()
	}
}
