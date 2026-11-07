// Package job keeps a process and everything it starts together on Windows,
// in a job object: their processor time is counted together and they are
// ended together. Windows does not group a process with its children
// otherwise; Unix has process groups for this, so the package is empty there.
package job
