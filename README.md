# Shinsakuto

Shinsakuto is a modular, asynchronous, distributed monitoring software 
written in **Go**. It is designed to manage large-scale IT infrastructures 
by strictly separating configuration, scheduling, execution, and notification.

## Component Architecture
The system is built on four pillars that communicate via REST APIs:

### 1. Arbiter (The Configurator)
The Arbiter serves as the single entry point for configuration.

Role: It reads inventory files (Hosts and Services) and synchronizes them across 
one or more Schedulers.

Action: It sends a global payload via the /v1/sync-all endpoint.

### 2. Scheduler (The Brain)
The Scheduler manages real-time state and the overall system intelligence.

Asynchronicity: It utilizes an internal queue (resultQueue) and a worker pool to 
process results without blocking network communications.

State: It maintains the status of entities in memory and periodically persists 
this data to a state_file.

Logic: It detects state changes (UP/DOWN/ALERT) and triggers the Reactionner or Broker.

### 3. Poller (The Executor)
The lightweight agent deployed on monitoring nodes.

Pull/Push: It retrieves a task via /v1/pop-task, executes it locally, and sends 
the result back via /v1/push-result.

Concurrency: It limits the number of simultaneous processes using a configurable 
semaphore system.

### 4. Reactionner (The Notifier)
The module dedicated to external communication.

Role: It receives notification requests from the Scheduler and executes defined 
actions such as Slack alerts, emails, or local scripts.

## Installation and Compilation

### Prerequisites
Go version 1.25+

Network access (HTTP) between components.

### Build
Use the provided Makefile to compile the entire suite:

```bash
make build
```
The binaries will be placed in the bin/ directory.

## Logging and Debugging
The logging system (pkg/logger) is optimized for performance:

Always: Logs critical events (start, stop, fatal errors) to both standard output 
and the log file.

Info (Debug Mode): If debug: true, it traces every command execution and network exchange. 
This is disabled by default to save on I/O.

History Log: The Scheduler maintains a dedicated file (history.log) for auditing
 state changes (e.g., HOST | srv-db-01 | DOWN | Connection refused).

## API Endpoints (Scheduler)

| Endpoint | Method | Consumer | Description |
| :--- | :--- | :--- | :--- |
| /v1/sync-all| POST | Arbiter | Bulk update of the inventory. | 
| /v1/pop-task | GET | Poller | Retrieval of a command to execute. | 
| /v1/push-result | POST | Poller | Asynchronous submission of a check result. |
| /v1/status | GET | CLI | Real-time global state visualization in JSON. |
