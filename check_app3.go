package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := "postgresql://neondb_owner:npg_jnyM3xQJ5lqO@ep-quiet-scene-b31gv8sg-pooler.c-4.ap-southeast-1.aws.neon.tech/neondb?sslmode=require&channel_binding=require"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to db: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Println("--- APPLICATION ---")
	row := db.QueryRowContext(ctx, `
		SELECT
			id, server_id, name, match_type, match_value, 
			monitor_port, monitor_http_url, log_source_type, log_source_path
		FROM applications
		WHERE id = 3;
	`)

	var id, serverID int64
	var name, matchType, matchValue string
	var monitorPort sql.NullInt64
	var monitorHTTPURL, logSourceType, logSourcePath sql.NullString

	err = row.Scan(&id, &serverID, &name, &matchType, &matchValue,
		&monitorPort, &monitorHTTPURL, &logSourceType, &logSourcePath)
	if err != nil {
		fmt.Printf("Error scanning application: %v\n", err)
	} else {
		fmt.Printf("id = %d\n", id)
		fmt.Printf("server_id = %d\n", serverID)
		fmt.Printf("name = %s\n", name)
		fmt.Printf("match_type = %s\n", matchType)
		fmt.Printf("match_value = %s\n", matchValue)

		if monitorPort.Valid {
			fmt.Printf("monitor_port = %d\n", monitorPort.Int64)
		} else {
			fmt.Printf("monitor_port = NULL\n")
		}

		if monitorHTTPURL.Valid {
			fmt.Printf("monitor_http_url = %s\n", monitorHTTPURL.String)
		} else {
			fmt.Printf("monitor_http_url = NULL\n")
		}

		if logSourceType.Valid {
			fmt.Printf("log_source_type = %s\n", logSourceType.String)
		} else {
			fmt.Printf("log_source_type = NULL\n")
		}

		if logSourcePath.Valid {
			fmt.Printf("log_source_path = %s\n", logSourcePath.String)
		} else {
			fmt.Printf("log_source_path = NULL\n")
		}
	}

	fmt.Println("\n--- LATEST METRICS ---")
	metricRow := db.QueryRowContext(ctx, `
		SELECT
			id, application_id, collected_at, status, primary_p_id, process_count,
			port_listening, http_available, http_status_code, active_response_avg_ms
		FROM application_metrics
		WHERE application_id = 3
		ORDER BY collected_at DESC
		LIMIT 1;
	`)

	var metricID, appID int64
	var collectedAt time.Time
	var status string
	var pid, processCount sql.NullInt64
	var portListening, httpAvailable sql.NullBool
	var httpStatusCode sql.NullInt32
	var httpRespTime sql.NullFloat64

	err = metricRow.Scan(&metricID, &appID, &collectedAt, &status, &pid, &processCount,
		&portListening, &httpAvailable, &httpStatusCode, &httpRespTime)

	if err != nil {
		if err == sql.ErrNoRows {
			fmt.Println("No metrics found for application_id = 3")
		} else {
			fmt.Printf("Error scanning metrics: %v\n", err)
		}
	} else {
		fmt.Printf("id = %d\n", metricID)
		fmt.Printf("application_id = %d\n", appID)
		fmt.Printf("collected_at = %s\n", collectedAt.Format(time.RFC3339))
		fmt.Printf("status = %s\n", status)

		if pid.Valid {
			fmt.Printf("pid = %d\n", pid.Int64)
		} else {
			fmt.Printf("pid = NULL\n")
		}

		if processCount.Valid {
			fmt.Printf("process_count = %d\n", processCount.Int64)
		} else {
			fmt.Printf("process_count = NULL\n")
		}

		if portListening.Valid {
			fmt.Printf("port_listening = %v\n", portListening.Bool)
		} else {
			fmt.Printf("port_listening = NULL\n")
		}

		if httpAvailable.Valid {
			fmt.Printf("http_available = %v\n", httpAvailable.Bool)
		} else {
			fmt.Printf("http_available = NULL\n")
		}

		if httpStatusCode.Valid {
			fmt.Printf("http_status_code = %d\n", httpStatusCode.Int32)
		} else {
			fmt.Printf("http_status_code = NULL\n")
		}

		if httpRespTime.Valid {
			fmt.Printf("active_http_response_time_ms = %f\n", httpRespTime.Float64)
		} else {
			fmt.Printf("active_http_response_time_ms = NULL\n")
		}
	}
}
