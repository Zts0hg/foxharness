/*
Package app defines UI-neutral user-entry commands, DTOs, ordered notification
mapping, and explicit interaction ports.

AgentRunner, RunCLI, RunTUI, and their concrete assembly helpers are a closed
compatibility facade for profiles not yet migrated. They are not extension
points and are deleted at migration boundary M24.
*/
package app
