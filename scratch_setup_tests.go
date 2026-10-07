//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func main() {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": 5,
		"email":   "user1@gmail.com",
		"role":    "USER",
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
	})

	tokenString, err := token.SignedString([]byte("vpspulse-super-secret-jwt-key-2026-auth-token-secure"))
	if err != nil {
		panic(err)
	}

	configs := []string{
		"https://google.com",
		"http://127.0.0.1:8080/",
		"http://10.0.0.1/",
		"http://invalid.invalid.endpoint/",
	}

	for _, url := range configs {
		reqBody := map[string]string{"url": url}
		body, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest("POST", "http://localhost:8080/api/v1/user/servers/1/health-configs", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+tokenString)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("Error creating config %s: %v\n", url, err)
			continue
		}

		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Printf("Created config %s: %d %s\n", url, resp.StatusCode, string(b))
	}
}
