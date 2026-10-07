//go:build ignore

package main

import (
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type User struct {
	ID    int64
	Email string
}

func main() {
	dsn := "postgresql://neondb_owner:npg_jnyM3xQJ5lqO@ep-quiet-scene-b31gv8sg-pooler.c-4.ap-southeast-1.aws.neon.tech/neondb?sslmode=require"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic(err)
	}

	var users []User
	db.Find(&users)

	for _, s := range users {
		fmt.Printf("User: ID=%d, Email=%s\n", s.ID, s.Email)
	}
}
