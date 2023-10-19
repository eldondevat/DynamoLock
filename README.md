# goTRC 
A DynamoDB backed distributed locking system written in go

---------

goTRC (short for time-release capsule) is a distributed locking implementation for
golang. It has a few qualities which differentiate it from some other implementations.

# Reached Goals
- Uses golang-aws-sdk-v2
- Leveled, structured logging using the accepted [slog] package.
- Context native: can use context cancellation to implement automatic release of held locks
- Substantial test coverage
- Built-in lock expiration and lock heartbeats to avoid zombie locks

# Future goals (Coming soon!)
- flock-style cli wrapper for commands, such as database migrations
- Automatic distributed testing suite

Alternatives:
- https://github.com/cirello-io/dynamolock/tree/master/v2
- https://github.com/Clever/dynamodb-lock-go
