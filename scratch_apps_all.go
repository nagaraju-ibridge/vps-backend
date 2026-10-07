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

	rows, err := db.QueryContext(ctx, "SELECT id, name, match_type, match_value FROM applications")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var name, matchType, matchValue string
		if err := rows.Scan(&id, &name, &matchType, &matchValue); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("App ID: %d, Name: %s, MatchType: %s, MatchValue: %s\n", id, name, matchType, matchValue)
	}
}
