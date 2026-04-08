package main

import (
	"log"

	"github.com/codecrafters-io/redis-starter-go/internal/redis/adapter/driven"
	"github.com/codecrafters-io/redis-starter-go/internal/redis/adapter/driver"
	"github.com/codecrafters-io/redis-starter-go/internal/redis/constant"
	"github.com/codecrafters-io/redis-starter-go/internal/redis/service"
)

func main() {
	store := driven.NewMemoryStore()
	svc := service.NewCommandService(store)
	srv := driver.NewServer(constant.DefaultPort, svc)
	log.Fatal(srv.ListenAndServe())
}
