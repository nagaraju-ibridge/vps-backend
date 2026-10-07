package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

func main() {
	loginPayload := `{"email":"admin@example.com","password":"AdminPassword123"}`
	resp, err := http.Post("http://localhost:8080/api/v1/auth/login", "application/json", bytes.NewBufferString(loginPayload))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var loginResp struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &loginResp); err != nil {
		log.Fatal("JSON Unmarshal error:", string(body))
	}
	token := loginResp.Data.Token

	req, _ := http.NewRequest("GET", "http://localhost:8080/api/v1/user/servers/1/processes?limit=100&sort=cpu_desc", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp2, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)
	
	// We want to find the agent process. Look for vpsmonitoring-agent
	var procs struct {
		Data struct {
			Processes []struct {
				PID         int    `json:"pid"`
				Name        string `json:"name"`
				Username    string `json:"username"`
				Cmdline     string `json:"cmdline"`
			} `json:"processes"`
		} `json:"data"`
	}
	json.Unmarshal(body2, &procs)

	for _, p := range procs.Data.Processes {
		if strings.Contains(p.Name, "vps") || strings.Contains(p.Name, "agent") {
			fmt.Printf("Agent Process: PID=%d, Name=%s, Username=%s, Cmdline=%s\n", p.PID, p.Name, p.Username, p.Cmdline)
		}
	}
	
	req3, _ := http.NewRequest("GET", "http://localhost:8080/api/v1/user/servers/1/systemd?limit=200", nil)
	req3.Header.Set("Authorization", "Bearer "+token)

	resp3, err := client.Do(req3)
	if err != nil {
		log.Fatal(err)
	}
	defer resp3.Body.Close()

	body3, _ := io.ReadAll(resp3.Body)
	
	var sys struct {
		Data struct {
			Services []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				LoadState   string `json:"load_state"`
				ActiveState string `json:"active_state"`
				SubState    string `json:"sub_state"`
			} `json:"services"`
		} `json:"data"`
	}
	json.Unmarshal(body3, &sys)

	for _, s := range sys.Data.Services {
		if strings.Contains(s.Name, "vps") || strings.Contains(s.Name, "agent") {
			fmt.Printf("Agent Service: Name=%s, LoadState=%s, ActiveState=%s, SubState=%s\n", s.Name, s.LoadState, s.ActiveState, s.SubState)
		}
	}
}
