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

	rows, err := db.QueryContext(ctx, "SELECT id, name, log_source_type, log_source_path FROM applications WHERE name = 'calculator'")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var name string
		var logType, logPath sql.NullString
		if err := rows.Scan(&id, &name, &logType, &logPath); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("App ID: %d, Name: %s, LogType: %v, LogPath: %v\n", id, name, logType, logPath)

		metricRow := db.QueryRowContext(ctx, `
			SELECT log_available, log_configured
			FROM application_metrics
			WHERE application_id = $1
			ORDER BY collected_at DESC LIMIT 1
		`, id)

		var logAvail sql.NullBool
		var logConf bool
		if err := metricRow.Scan(&logAvail, &logConf); err != nil {
			fmt.Println("No metrics or error:", err)
		} else {
			fmt.Printf("Metric LogConfigured: %v, LogAvailable: %v\n", logConf, logAvail)
		}
	}
}
