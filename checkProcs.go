package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

// Exit codes for monitoring Sensu/Nagios
const (
	// OK will return 0
	OK = 0

	// WARNING will return 1
	WARNING = 1

	// CRITICAL will return 2
	CRITICAL = 2

	// UNKNOWN will return 3
	UNKNOWN = 3
)

// Process is the generic interface that is implemented on every platform
// and provides common operations for processes.
type Process interface {
	// Pid is the process ID for this process.
	Pid() int

	// PPid is the parent process ID for this process.
	PPid() int

	// Executable name running this process. This is not a path to the
	// executable.
	Executable() string
}

// Processes returns all processes.
//
// This of course will be a point-in-time snapshot of when this method was
// called. Some operating systems don't provide snapshot capability of the
// process table, in which case the process table returned might contain
// ephemeral entities that happened to be running when this was called.
//
// Example:
// procs, _ := Processes()
//
func Processes() ([]Process, error) {
	return processes()
}

// FindProcess looks up a single process by pid.
//
// Process will be nil and error will be nil if a matching process is
// not found.
//
// Example:
// foo, _ := FindProcess(4256)
// fmt.Println(foo.Executable(), foo.Pid())
// for _, i := range procs {
//   if i.Executable()
func FindProcess(pid int) (Process, error) {
	return findProcess(pid)
}

// UnixProcess is an implementation of Process that contains Unix-specific
// fields and information.
type UnixProcess struct {
	pid   int
	ppid  int
	state rune
	pgrp  int
	sid   int

	binary string
}

// Pid simply return pid of the specific process
func (p *UnixProcess) Pid() int {
	return p.pid
}

// PPid simply return ppid of the specific process
func (p *UnixProcess) PPid() int {
	return p.ppid
}

// Executable simply return name of the process
func (p *UnixProcess) Executable() string {
	return p.binary
}

// findProcess is returning all information about the specific process
func findProcess(pid int) (Process, error) {
	dir := fmt.Sprintf("/proc/%d", pid)
	_, err := os.Stat(dir)
	if err != nil {
		// file does not exist
		if os.IsNotExist(err) {
			return nil, nil
		}

		// other error, if any
		return nil, err
	}

	return newUnixProcess(pid)
}

// newUnixProcess is adding pid to the Process type
// and call Refresh function to fill missing data
func newUnixProcess(pid int) (*UnixProcess, error) {
	p := &UnixProcess{pid: pid}
	return p, p.Refresh()
}

// Refresh reloads all the data associated with this process.
func (p *UnixProcess) Refresh() error {
	statPath := fmt.Sprintf("/proc/%d/stat", p.pid)
	dataBytes, err := ioutil.ReadFile(statPath)
	if err != nil {
		return err
	}

	// First, parse out the image name
	data := string(dataBytes)
	binStart := strings.IndexRune(data, '(') + 1
	binEnd := strings.IndexRune(data[binStart:], ')')

	// setup name of the proces on to the pointer
	p.binary = data[binStart : binStart+binEnd]

	// Move past the image name and start parsing the rest
	data = data[binStart+binEnd+2:]

	// setup rest of the process types in the pointer
	// and return error if any
	_, err = fmt.Sscanf(data,
		"%c %d %d %d",
		&p.state,
		&p.ppid,
		&p.pgrp,
		&p.sid)

	return err
}

// processes return all unix processes as a struct of Process type
func processes() ([]Process, error) {
	d, err := os.Open("/proc")
	if err != nil {
		return nil, err
	}
	defer d.Close()

	results := make([]Process, 0, 50)
	for {
		// Readdir(10) return slice of first 10 processes
		// if in for{} going for the next 10 processes till return al of them
		fis, err := d.Readdir(10)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		// now we need to iterate over the slice of first 10 processes
		// we can call their names by .Name() as it's interface FileInfo
		// we need to use for as we don't know how many processes there is
		// at the /proc diretory, so it's better to get 10 parse and get another 10
		for _, fi := range fis {
			// We only care about directories, since all pids are dirs
			if !fi.IsDir() {
				continue
			}

			// We only care if the name starts with a numeric
			name := fi.Name()
			if name[0] < '0' || name[0] > '9' {
				continue
			}

			// From this point forward, any errors we just ignore, because
			// it might simply be that the process doesn't exist anymore.
			// convert string to int
			pid, err := strconv.ParseInt(name, 10, 0)
			if err != nil {
				continue
			}

			p, err := newUnixProcess(int(pid))
			if err != nil {
				continue
			}

			results = append(results, p)
		}
	}

	return results, nil
}

// Main function to actually start programm
func main() {
	run, command, err := Command()
	if err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(UNKNOWN)
	}

	for _, i := range run {
		if i.Executable() == command && i.PPid() == 1 {
			fmt.Printf("Process exist: %v, pid: %d", i.Executable(), i.Pid())
			os.Exit(OK)
		}
	}

	fmt.Printf("Process do not exist: %v", command)
	os.Exit(CRITICAL)
}

// Command function is checking if we run it with the correct
// parameters and return process information
func Command() ([]Process, string, error) {
	if len(os.Args) != 3 {
		help()
	}
	if os.Args[1] != "-c" {
		help()
	}

	command := os.Args[2]
	if command == "" {
		help()
	}

	procs, err := Processes()
	if err != nil {
		return nil, "", err
	}
	return procs, command, nil
}

// help function to show help how to execute
// this script
func help() {
	fmt.Println("")
	fmt.Println("  -c string")
	fmt.Println("    	process name (string)")
	fmt.Println("")
	fmt.Println("  example:")
	fmt.Println("       ./check_proc -c \"sshd\"")
	fmt.Println("")
	os.Exit(UNKNOWN)
}
