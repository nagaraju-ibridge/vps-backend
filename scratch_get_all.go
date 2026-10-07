package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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
	json.Unmarshal(body, &loginResp)
	token := loginResp.Data.Token

	req, _ := http.NewRequest("GET", "http://localhost:8080/api/v1/user/servers/1/processes?limit=1000", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp2, _ := client.Do(req)
	
	body2, _ := io.ReadAll(resp2.Body)
	
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
		fmt.Printf("Process: %s (%s)\n", p.Name, p.Username)
	}
	
	req3, _ := http.NewRequest("GET", "http://localhost:8080/api/v1/user/servers/1/systemd?limit=1000", nil)
	req3.Header.Set("Authorization", "Bearer "+token)

	resp3, _ := client.Do(req3)
	body3, _ := io.ReadAll(resp3.Body)
	
	var sys struct {
		Data struct {
			Services []struct {
				Name        string `json:"name"`
			} `json:"services"`
		} `json:"data"`
	}
	json.Unmarshal(body3, &sys)

	for _, s := range sys.Data.Services {
		fmt.Printf("Service: %s\n", s.Name)
	}
}
