// Package gomath builds deterministic mathematical animation scenes.
//
// Geometry is expressed in world units with +Y pointing up. Objects are mutable
// while authoring; Scene records independent snapshots when Play or Wait is
// called. Sampling a completed scene does not mutate the authoring objects.
// Render and export live in separate packages so the core has no I/O dependency.
package gomath
