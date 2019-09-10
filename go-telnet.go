package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

func safeScan(scanner *bufio.Scanner, out chan string) {

	for scanner.Scan() {
		out <- scanner.Text()
	}
	err := scanner.Err()
	if err != nil {
		log.Printf("Scanner error %s\n", err)
	}
	close(out)
}

func readRoutine(ctx context.Context, conn net.Conn) {
	scanner := bufio.NewScanner(conn)
	netChan := make(chan string)
	go safeScan(scanner, netChan)
OUTER:
	for {
		select {
		case <-ctx.Done():
			log.Printf("readRoutine ctx done\n")
			break OUTER
		case str, ok := <-netChan:
			if !ok {
				log.Printf("Net chan closed\n")
				break OUTER
			}

			log.Printf("From server: %s", str)
		}
	}
	log.Printf("Finished readRoutine")
}

func writeRoutine(ctx context.Context, conn net.Conn) {
	scanner := bufio.NewScanner(os.Stdin)
	stdioChan := make(chan string)
	go safeScan(scanner, stdioChan)
OUTER:
	for {
		select {
		case <-ctx.Done():
			log.Printf("writeRoutine ctx done\n")
			break OUTER
		case str, ok := <-stdioChan:
			if !ok {
				log.Printf("Stdio chan closed\n")
				break OUTER
			}
			log.Printf("To server %v\n", str)

			_, err := conn.Write([]byte(fmt.Sprintf("%s\n", str)))
			if err != nil {
				log.Println("Write error", err)
			}
		}

	}
	log.Printf("Finished writeRoutine")
}

func Telnet(server, port string, timeout int) {
	dialer := &net.Dialer{}
	ctx := context.Background()
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}

	conn, err := dialer.DialContext(ctx, "tcp", server+":"+port)
	if err != nil {
		log.Fatalf("Cannot connect: %v", err)
	}

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		readRoutine(ctx, conn)
		cancel()
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		writeRoutine(ctx, conn)
		cancel()
		wg.Done()
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		fmt.Println()
		fmt.Println(sig)
		cancel()
	}()

	wg.Wait()
	conn.Close()
}

func main() {

	var rootCmd = &cobra.Command{
		Use:   "go-telnet <server> [port] [timeout]",
		Short: "Establishes network connection",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			server := args[0]
			port := "23"
			if len(args) > 1 {
				port = args[1]
			}

			var timeout int
			if len(args) > 2 {
				timeout, _ = strconv.Atoi(args[2])
			}

			Telnet(server, port, timeout)
		},
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

}
