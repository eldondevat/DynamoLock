package infra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/exp/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
	"github.com/google/uuid"
)

type lock struct {
	name    string
	timeout time.Duration
}

type Locker struct {
	ticker            *time.Ticker
	HeartbeatInterval time.Duration
	client            *dynamodb.Client
	lockerId          string
	ctx               context.Context
	cancel            context.CancelFunc
	lockTable	  string
	locksHeld         []lock
	releaser          chan string
	recorder          chan lock
}

func NewLocker(client *dynamodb.Client, ctx context.Context, lockTable string) *Locker {
	innerCtx, cancel := context.WithCancel(ctx)
	id := uuid.New().String()
	newLocker := Locker{time.NewTicker(1 * time.Minute),
		1 * time.Minute,
		client,
		id,
		innerCtx,
		cancel,
		lockTable,
		nil,
		make(chan string),
		make(chan lock),
	}
	go newLocker.heartBeater(ctx) // We use the original context here in case we are shutting down the inner context
	return &newLocker
}

func (l *Locker) refresh(ctx context.Context) {
	for _, lock := range l.locksHeld {
		ok, err := l.AcquireLock(lock.name, lock.timeout)
		if !ok || err != nil {
			panic(fmt.Errorf("lock %s held by %s could not be refreshed : %w", lock.name, l.lockerId, err))
		}
	}
}

func (l *Locker) heartBeater(ctx context.Context) {
	for {
		select {
		case <-l.ticker.C:
			slog.Debug("Tick refresh")
			l.refresh(ctx)
		case toRelease := <-l.releaser:
			slog.Debug("Lock release")
			l.releaseLock(ctx, toRelease)
		case toRecord := <-l.recorder:
			slog.Debug("Lock record", slog.String("lockname", toRecord.name))
			l.locksHeld = append(l.locksHeld, toRecord)
			if toRecord.timeout < l.HeartbeatInterval {
				l.HeartbeatInterval = toRecord.timeout / 2
				l.ticker.Reset(l.HeartbeatInterval)
				l.refresh(ctx)
			}
		case <-l.ctx.Done():
			for _, lock := range l.locksHeld {
				l.releaseLock(ctx, lock.name)
			}
			close(l.releaser)
			close(l.recorder)
			return
		}
	}
}

func (l *Locker) Close() {
	l.cancel()
}

func (l *Locker) releaseLock(ctx context.Context, name string) {
	_, err := l.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		Key: map[string]dynamodbtypes.AttributeValue{
			"name": &dynamodbtypes.AttributeValueMemberS{Value: name},
		},
		ConditionExpression: aws.String("lockerId = :lockerId"),
		ExpressionAttributeValues: map[string]dynamodbtypes.AttributeValue{
			":lockerId": &dynamodbtypes.AttributeValueMemberS{Value: l.lockerId},
		},
		TableName: aws.String(l.lockTable),
	})
	var updatedLocksHeld []lock
	if err != nil {
		panic(fmt.Errorf("lock %s held by %s could not be released : %w", name, l.lockerId, err))
	}

	if err == nil {
		for _, existingLock := range l.locksHeld {
			if existingLock.name != name {
				updatedLocksHeld = append(updatedLocksHeld, existingLock)
			}
		}
	}
	l.locksHeld = updatedLocksHeld
}

func (l *Locker) ReleaseLock(name string) {
	l.releaser <- name
}

func (l *Locker) AcquireLock(name string, timeout time.Duration) (bool, error) {
	held := false
	for _, heldLock := range l.locksHeld {
		if heldLock.name == name {
			held = true
			break
		}
	}
	slog.Debug("Attempting to acquire lock", "locker", l.lockerId, "name", name, "held", held)
	out, err := l.client.UpdateItem(l.ctx, &dynamodb.UpdateItemInput{
		Key: map[string]dynamodbtypes.AttributeValue{
			"name": &dynamodbtypes.AttributeValueMemberS{Value: name},
		},
		UpdateExpression:    aws.String("SET lockerId = :lockerId, ExpireAt = :expiry"),
		ConditionExpression: aws.String("attribute_not_exists(lockerId) or lockerId = :lockerId or :now > ExpireAt"),
		ReturnValues:        dynamodbtypes.ReturnValueUpdatedNew,
		ExpressionAttributeValues: map[string]dynamodbtypes.AttributeValue{
			":lockerId": &dynamodbtypes.AttributeValueMemberS{Value: l.lockerId},
			":now":      &dynamodbtypes.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Unix())},
			":expiry":   &dynamodbtypes.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(timeout).Unix())},
		},
		TableName: aws.String(l.lockTable),
	})
	x, _ := json.Marshal(out)
	slog.Debug("update result:", "result", string(x))
	if err == nil {
		if !held {
			l.recorder <- lock{name, timeout}
		}
	} else {
		var oe *smithy.OperationError
		if errors.As(err, &oe) && strings.Contains(oe.Error(), "ConditionalCheckFailedException") {
			return false, nil
		} else {
			return false, err
		}
	}

	return true, nil
}
