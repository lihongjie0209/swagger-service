//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/lihongjie0209/swagger-service/internal/cache"
	goredis "github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	rediscontainer "github.com/testcontainers/testcontainers-go/modules/redis"
)

func TestRedisLock(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	container, err := rediscontainer.Run(ctx, "redis:7.4-alpine")
	if err != nil {
		t.Fatal(err)
	}
	testcontainers.CleanupContainer(t, container)
	connectionString, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	options, err := goredis.ParseURL(connectionString)
	if err != nil {
		t.Fatal(err)
	}
	client := goredis.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })

	locker := cache.NewLocker(client)
	lock, acquired, err := locker.TryLock(ctx, "integration", 10*time.Second)
	if err != nil || !acquired {
		t.Fatalf("first lock acquired=%v err=%v", acquired, err)
	}
	_, secondAcquired, err := locker.TryLock(ctx, "integration", 10*time.Second)
	if err != nil || secondAcquired {
		t.Fatalf("competing lock acquired=%v err=%v", secondAcquired, err)
	}
	if err := lock.Unlock(ctx); err != nil {
		t.Fatal(err)
	}
}
