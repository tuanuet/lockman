package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func TestStrictFencedWriteOutput(t *testing.T) {
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run failed: %v", err)
	}
	defer redisServer.Close()

	var out bytes.Buffer
	client := goredis.NewClient(&goredis.Options{Addr: redisServer.Addr()})
	defer client.Close()

	if err := run(&out, client); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "fencing token increased") {
		t.Fatalf("unexpected output: %s", output)
	}
}
