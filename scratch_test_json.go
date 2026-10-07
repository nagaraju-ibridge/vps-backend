package main

import (
	"encoding/json"
	"fmt"

	"vpsmonitoring-backend/internal/application/dto"
)

func main() {
	j := `{"application_id": 7, "observation_state": "MATCHED", "log_configured": true, "log_available": true}`
	var entry dto.ApplicationTelemetryEntry
	err := json.Unmarshal([]byte(j), &entry)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	if entry.LogAvailable != nil {
		fmt.Printf("LogAvailable: %v\n", *entry.LogAvailable)
	} else {
		fmt.Println("LogAvailable is nil")
	}
}
