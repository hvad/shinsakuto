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

## Container

### Build shinsakuto image from Dockerfile

```bash
container build -t shinsakuto:latest .
```

### Run image

```bash
container run -it -p 8080:8080 -p 8090:8090 \
-v "$(pwd)/etc/shinsakuto/standalone:/shinsakuto/etc" \
-v "$(pwd)/etc/shinsakuto/conf.d:/shinsakuto/etc/shinsakuto/conf.d" \
-v "$(pwd)/var/lib/scheduler:/shinsakuto/var/lib/scheduler" shinsakuto
```

## Monitoring Configuration Example
This section provides a complete example of how to configure a host and 
its associated services using Shinsakuto's inheritance system and macro substitutions.

### 1. Command Definitions (commands.yml)
Commands define how the Poller executes check scripts. Use the **$ADDRESS$** macro,
which the Poller replaces with the host's actual IP at runtime.

```yaml
commands:
  - id: check_ping
    # The poller replaces $ADDRESS$ with the host's actual IP
	command_line: "/sbin/ping -c 3 $ADDRESS$ > /dev/null && echo 0 || echo 2"

  - id: check_http
    # Example of an inline web check
    command_line: "/usr/bin/curl -Is http://$ADDRESS$ | head -n 1 > /dev/null && echo 0 || echo 2"
```

### 2. Object Templates (templates.yml)
Templates allow you to define shared parameters. 
Objects with register: false are used for inheritance only and are not actively monitored.

```yaml
hosts:
  - id: generic-linux-server
    check_command: check_ping
    check_period: 24x7
    contacts:
      - admin
    register: false # This is a template

services:
  - id: generic-service
    check_period: 24x7
    contacts:
      - admin
    register: false # This is a template
```

### 3. Active Host and Services (localhost.yml)
This file defines the actual monitoring targets by inheriting from the templates above.

```yaml
# Host Definition
hosts:
  - id: web-server-prod
    use: generic-linux-server  # Inherits check_command and contacts
    address: 192.168.1.50      # Target IP for $ADDRESS$ macro
    register: true             # Enable active monitoring

# Services attached to the Host
services:
  - id: HTTP_Status
    use: generic-service
    host_name: web-server-prod # Link to the host defined above
    check_command: check_http
    register: true

  - id: ICMP_Latency
    use: generic-service
    host_name: web-server-prod
    check_command: check_ping
    register: true
```

### 4. Support Objects (contacts.yml)
Essential definitions contact .

```yaml
contacts:
  - id: admin
    email: admin@example.com # Destination for SMTP notifications
```

### 5. Support Objects (timeperiods.yml)
Timeperiod definition for checks and notification.

```yaml
timeperiods:
  - id: 24x7
    monday: ["00:00-24:00"]
    tuesday: ["00:00-24:00"]
    wednesday: ["00:00-24:00"]
    thursday: ["00:00-24:00"]
    friday: ["00:00-24:00"]
    saturday: ["00:00-24:00"]
    sunday: ["00:00-24:00"]
```
