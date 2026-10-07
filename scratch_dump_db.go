//go:build ignore

package main

import (
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Server struct {
	ID     int64
	UserID int64
	Name   string
}

func main() {
	dsn := "postgresql://neondb_owner:npg_jnyM3xQJ5lqO@ep-quiet-scene-b31gv8sg-pooler.c-4.ap-southeast-1.aws.neon.tech/neondb?sslmode=require"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic(err)
	}

	var servers []Server
	db.Find(&servers)

	for _, s := range servers {
		fmt.Printf("Server: ID=%d, UserID=%d, Name=%s\n", s.ID, s.UserID, s.Name)
	}
}
