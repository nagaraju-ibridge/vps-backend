package main

import (
	"context"
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"log"
	"vpsmonitoring-backend/internal/application/dto"
	"vpsmonitoring-backend/internal/application/service"
)

type mockAppRepo struct {
	db *gorm.DB
}

func main() {
	dsn := "postgresql://neondb_owner:npg_jnyM3xQJ5lqO@ep-quiet-scene-b31gv8sg-pooler.c-4.ap-southeast-1.aws.neon.tech/neondb?sslmode=require"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	svc := service.NewApplicationService(db, nil)

	req := dto.ApplicationCreateRequest{
		Name:        "TestApp",
		MatchType:   "systemd_unit",
		MatchValue:  "test.service",
		MonitorPort: 9091,
	}

	app, err := svc.Create(context.Background(), 100, 1, req)
	if err != nil {
		log.Fatalf("Create failed: %v", err)
	}

	fmt.Printf("Created App ID: %d, MonitorPort: %v\n", app.ID, app.MonitorPort)

	// Fetch directly from DB
	var port *int
	err = db.Raw("SELECT monitor_port FROM applications WHERE id = ?", app.ID).Scan(&port).Error
	if err != nil {
		log.Fatal(err)
	}

	if port != nil {
		fmt.Printf("DB MonitorPort: %d\n", *port)
	} else {
		fmt.Printf("DB MonitorPort: NULL\n")
	}
}
