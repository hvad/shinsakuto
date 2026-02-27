# Shinsakuto

A distributed, high-performance monitoring system written in **Golang**, 
inspired by the Shinken/Nagios architecture. 
This project implements a microservices-oriented approach where each 
monitoring task is handled by a dedicated, independent process.

## System Architecture

The project is split into four decoupled components that communicate via REST APIs:

* **Arbiter**: The "Dispatcher." It reads local configurations and synchronizes the 
desired state (Hosts & Services) with the Scheduler.
* **Scheduler**: The "Brain." It manages the state machine (**Soft vs. Hard states**), 
handles check timing logic, and maintains the current status registry.
* **Poller**: The "Muscle." Stateless workers that pull tasks from the Scheduler and 
execute them via the system shell.
* **Reactionner**: The "Voice." A passive listener that waits for the Scheduler to send 
notification requests (alerts or recoveries).



