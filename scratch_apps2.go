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

	metricRow := db.QueryRowContext(ctx, `
		SELECT log_available, log_configured, log_error_class
		FROM application_metrics
		WHERE application_id = 7
		ORDER BY collected_at DESC LIMIT 1
	`)

	var logAvail sql.NullBool
	var logConf bool
	var logErrClass sql.NullString
	if err := metricRow.Scan(&logAvail, &logConf, &logErrClass); err != nil {
		fmt.Println("No metrics or error:", err)
	} else {
		fmt.Printf("Metric LogConfigured: %v, LogAvailable: %v, LogErrorClass: %v\n", logConf, logAvail, logErrClass)
	}
}
