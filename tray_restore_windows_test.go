package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestTrayRestoreAcrossProcesses(t *testing.T) {
	const classEnv = "EVELA_TEST_TRAY_RESTORE_CLASS"
	if name := os.Getenv(classEnv); name != "" {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		trayRestoreClassName = windows.StringToUTF16Ptr(name)
		restored := false
		stop := listenForTrayRestore(func() { restored = true })
		if stop == nil {
			t.Fatal("could not create the tray restore listener")
		}
		defer stop()
		fmt.Println("tray ready")

		// PeekMessage dispatches synchronous messages to this thread's windows.
		peekMessage := windowUser32.NewProc("PeekMessageW")
		var message [8]uintptr // Aligned storage for a Win32 MSG on 32- and 64-bit Windows.
		for !restored {
			peekMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0, 0)
			time.Sleep(5 * time.Millisecond)
		}
		return
	}

	name := fmt.Sprintf("%s.TestTrayRestore.%d", applicationID, os.Getpid())
	class := windows.StringToUTF16Ptr(name)
	if requestTrayRestore(class) {
		t.Fatal("found a listener before starting the tray process")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTrayRestoreAcrossProcesses$")
	child.Env = append(os.Environ(), classEnv+"="+name)
	child.Stderr = os.Stderr
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		if child.ProcessState == nil {
			_ = child.Wait()
		}
	}()
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || line != "tray ready\n" {
		t.Fatalf("tray process did not expose its listener: %q, %v", line, err)
	}
	if !requestTrayRestore(class) {
		t.Fatal("the tray process did not acknowledge a restore request")
	}
	if err := child.Wait(); err != nil {
		t.Fatal(err)
	}
	if requestTrayRestore(class) {
		t.Fatal("a restore request succeeded after the listener closed")
	}
}
