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

	var collectedAt time.Time
	err = db.QueryRowContext(ctx, "SELECT collected_at FROM application_metrics WHERE application_id = 7 ORDER BY collected_at DESC LIMIT 1").Scan(&collectedAt)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Collected At: %v\n", collectedAt)
}
