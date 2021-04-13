package main

import (
	"os"

	"github.com/Vincent20101/mongo-go-driver/benchmark"
)

func main() {
	os.Exit(benchmark.DriverBenchmarkMain())
}
