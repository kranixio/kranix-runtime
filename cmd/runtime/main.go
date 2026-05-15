package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/kranix-io/kranix-runtime/config"
	"github.com/kranix-io/kranix-runtime/internal/registry"
	"google.golang.org/grpc"
)

func main() {
	configPath := flag.String("config", "./config/local.yaml", "Path to configuration file")
	grpcPort := flag.Int("grpc-port", 50051, "gRPC server port")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Get the configured runtime driver
	driver, err := registry.Get(cfg.Runtime.DefaultBackend, cfg)
	if err != nil {
		log.Fatalf("Failed to initialize driver %q: %v", cfg.Runtime.DefaultBackend, err)
	}

	log.Printf("Using runtime backend: %s", driver.Backend())

	// Verify driver connectivity
	if err := driver.Ping(context.Background()); err != nil {
		log.Fatalf("Driver ping failed: %v", err)
	}

	// Setup gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *grpcPort))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()

	// TODO: Register runtime gRPC service
	// runtimepb.RegisterRuntimeServer(grpcServer, server.NewRuntimeServer(driver))

	// Start gRPC server
	go func() {
		log.Printf("Starting gRPC server on port %d", *grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	grpcServer.GracefulStop()
	log.Println("Shutdown complete")
}
