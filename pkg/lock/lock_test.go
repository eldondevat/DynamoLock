package infra

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/google/uuid"
	"golang.org/x/exp/slog"

	"github.com/stretchr/testify/assert"
)

func init() {
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(h))

}

func TestLock(t *testing.T) {
	testLock := uuid.New().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	awsConf, err := config.LoadDefaultConfig(ctx)
	assert.Nil(t, err, "error should be nil")

	n := NewLocker(dynamodb.NewFromConfig(awsConf), ctx)
	ok, err := n.AcquireLock(testLock, time.Second*10)
	assert.True(t, ok, "lock should be acquired")
	assert.Nil(t, err, "error should be nil")
}

func TestFailToGetAcquiredLock(t *testing.T) {
	testLock := uuid.New().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	awsConf, err := config.LoadDefaultConfig(ctx)
	assert.Nil(t, err, "error should be nil")

	n := NewLocker(dynamodb.NewFromConfig(awsConf), ctx)
	ok, err := n.AcquireLock(testLock, time.Second*10)
	assert.True(t, ok, "lock should be acquired")
	assert.Nil(t, err, "error should be nil")

	b := NewLocker(dynamodb.NewFromConfig(awsConf), ctx)
	ok, err = b.AcquireLock(testLock, time.Second*10)
	assert.Nil(t, err, "error should be nil")
	assert.False(t, ok, "lock should not be acquired")
}

func TestGetExpiredAcquiredLock(t *testing.T) {
	testLock := uuid.New().String()
	log.Printf(testLock)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	awsConf, err := config.LoadDefaultConfig(ctx)
	assert.Nil(t, err, "error should be nil")

	n := NewLocker(dynamodb.NewFromConfig(awsConf), ctx)
	ok, err := n.AcquireLock(testLock, time.Second*1)
	assert.True(t, ok, "lock should be acquired")
	assert.Nil(t, err, "error should be nil")
	t.Logf("Stopping ticker %+v", n.ticker)
	time.Sleep(1 * time.Second)
	n.ticker.Stop()

	time.Sleep(2 * time.Second)
	b := NewLocker(dynamodb.NewFromConfig(awsConf), ctx)
	ok, err = b.AcquireLock(testLock, time.Second*10)
	assert.Nil(t, err, "error should be nil")
	assert.True(t, ok, "lock should be acquired")
}

func TestGetReleasedLock(t *testing.T) {
	testLock := uuid.New().String()
	log.Printf(testLock)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	awsConf, err := config.LoadDefaultConfig(ctx)
	assert.Nil(t, err, "error should be nil")

	n := NewLocker(dynamodb.NewFromConfig(awsConf), ctx)
	ok, err := n.AcquireLock(testLock, time.Second*10)
	assert.True(t, ok, "lock should be acquired")
	assert.Nil(t, err, "error should be nil")
	n.ReleaseLock(testLock)

	b := NewLocker(dynamodb.NewFromConfig(awsConf), ctx)
	ok, err = b.AcquireLock(testLock, time.Second*10)
	assert.Nil(t, err, "error should be nil")
	assert.True(t, ok, "lock should be acquired")
}

func TestReaquireHeldLock(t *testing.T) {
	testLock := uuid.New().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	awsConf, err := config.LoadDefaultConfig(ctx)
	assert.Nil(t, err, "error should be nil")

	n := NewLocker(dynamodb.NewFromConfig(awsConf), ctx)
	ok, err := n.AcquireLock(testLock, time.Second*10)
	assert.True(t, ok, "lock should be acquired")
	assert.Nil(t, err, "error should be nil")
	n.ReleaseLock(testLock)

	ok, err = n.AcquireLock(testLock, time.Second*10)
	assert.Nil(t, err, "error should be nil")
	assert.True(t, ok, "lock should be acquired")
}
