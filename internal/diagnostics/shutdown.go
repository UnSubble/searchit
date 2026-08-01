package diagnostics

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

type GoRoutine struct {
	ID       string
	State    string
	Stack    []string
	TopFunc  string
	File     string
	Line     string
	Blocking string
}

func RunDiagnostics(timeout time.Duration, scanCtxErr, drainCtxErr error) {
	time.Sleep(timeout)
	fmt.Fprintf(os.Stderr, "\n=== SHUTDOWN DIAGNOSTIC (stuck > %v) ===\n", timeout)

	buf := make([]byte, 5*1024*1024)
	n := runtime.Stack(buf, true)
	stackStr := string(buf[:n])

	fmt.Fprintln(os.Stderr, "1. FULL GOROUTINE DUMP")
	fmt.Fprintln(os.Stderr, "----------------------")
	fmt.Fprintln(os.Stderr, stackStr)
	fmt.Fprintln(os.Stderr, "----------------------")

	fmt.Fprintln(os.Stderr, "\n2. STRUCTURED SHUTDOWN STATE")
	fmt.Fprintln(os.Stderr, "----------------------------")

	// Parse goroutines
	blocks := strings.Split(stackStr, "\n\n")
	var routines []GoRoutine

	for _, block := range blocks {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) < 2 {
			continue
		}

		header := lines[0]
		if !strings.HasPrefix(header, "goroutine ") {
			continue
		}

		// "goroutine 1 [chan receive]:"
		parts := strings.SplitN(header[10:], " ", 2)
		if len(parts) != 2 {
			continue
		}
		id := parts[0]
		state := strings.Trim(parts[1], "[]:")

		gr := GoRoutine{
			ID:    id,
			State: state,
			Stack: lines[1:],
		}

		if len(lines) > 1 {
			gr.TopFunc = strings.TrimSpace(lines[1])
			if len(lines) > 2 {
				fileLine := strings.TrimSpace(lines[2])
				if idx := strings.LastIndex(fileLine, ":"); idx != -1 {
					gr.File = fileLine[:idx]
					gr.Line = fileLine[idx+1:]
				}
			}
		}

		// Determine blocking primitive
		switch {
		case strings.Contains(state, "chan receive"):
			gr.Blocking = "channel receive"
		case strings.Contains(state, "chan send"):
			gr.Blocking = "channel send"
		case strings.Contains(state, "select"):
			gr.Blocking = "select"
		case strings.Contains(state, "IO wait"):
			gr.Blocking = "runtime_pollWait (IO)"
		case strings.Contains(state, "semacquire"):
			if containsFunc(gr.Stack, "sync.(*WaitGroup).Wait") {
				gr.Blocking = "WaitGroup.Wait"
			} else if containsFunc(gr.Stack, "sync.(*Mutex).Lock") {
				gr.Blocking = "mutex"
			} else if containsFunc(gr.Stack, "sync.(*Cond).Wait") {
				gr.Blocking = "Cond.Wait"
			} else {
				gr.Blocking = "semaphore acquire"
			}
		default:
			gr.Blocking = state
		}

		routines = append(routines, gr)
	}

	// Engine stats
	var workers, activeWorkers int
	var waitGroups []string

	// Recursion stats
	var recManager *GoRoutine

	// Progress stats
	var progManager *GoRoutine

	// Producer stats
	var producer *GoRoutine
	var mainRoutine *GoRoutine

	for i := range routines {
		gr := &routines[i]
		if containsFunc(gr.Stack, "engine.Worker") || containsFunc(gr.Stack, "fuzz.Worker") {
			workers++
			if !strings.Contains(gr.Blocking, "chan receive") && !strings.Contains(gr.TopFunc, "delay") {
				activeWorkers++
			}
		}
		if gr.Blocking == "WaitGroup.Wait" {
			waitGroups = append(waitGroups, fmt.Sprintf("goroutine %s waiting in %s", gr.ID, gr.TopFunc))
		}
		if containsFunc(gr.Stack, "recursion.(*Manager).Run") {
			recManager = gr
		}
		if containsFunc(gr.Stack, "progress.(*Manager).Start") {
			progManager = gr
		}
		if containsFunc(gr.Stack, "wordlist.(*Producer)") || containsFunc(gr.Stack, "wordlist.Producer") || containsFunc(gr.Stack, "cmd.runFuzz.func") {
			if containsFunc(gr.Stack, "Produce") || containsFunc(gr.Stack, "wordlist") {
				producer = gr
			}
		}
		if containsFunc(gr.Stack, "main.main") {
			mainRoutine = gr
		}
	}

	fmt.Fprintln(os.Stderr, "Engine")
	fmt.Fprintln(os.Stderr, "------")
	fmt.Fprintf(os.Stderr, "- worker count: %d\n", workers)
	fmt.Fprintf(os.Stderr, "- active worker count: %d\n", activeWorkers)
	if len(waitGroups) > 0 {
		fmt.Fprintf(os.Stderr, "- WaitGroup status: BLOCKED (%s)\n", strings.Join(waitGroups, ", "))
	} else {
		fmt.Fprintln(os.Stderr, "- WaitGroup status: CLEAR")
	}
	fmt.Fprintln(os.Stderr, "- jobs channel: <see stack for state>")
	fmt.Fprintln(os.Stderr, "- results channel: <see stack for state>")
	fmt.Fprintln(os.Stderr, "")

	fmt.Fprintln(os.Stderr, "Recursion Manager")
	fmt.Fprintln(os.Stderr, "-----------------")
	if recManager != nil {
		fmt.Fprintf(os.Stderr, "- current state: BLOCKED on %s\n", recManager.Blocking)
		fmt.Fprintf(os.Stderr, "- currently waiting on what? %s\n", recManager.TopFunc)
	} else {
		fmt.Fprintln(os.Stderr, "- current state: EXITED or NOT RUNNING")
	}
	fmt.Fprintln(os.Stderr, "")

	fmt.Fprintln(os.Stderr, "Progress Manager")
	fmt.Fprintln(os.Stderr, "----------------")
	if progManager != nil {
		fmt.Fprintln(os.Stderr, "- running? YES")
		fmt.Fprintf(os.Stderr, "- waiting on which select branch? %s (func: %s)\n", progManager.Blocking, progManager.TopFunc)
	} else {
		fmt.Fprintln(os.Stderr, "- running? NO (exited)")
	}
	fmt.Fprintln(os.Stderr, "")

	fmt.Fprintln(os.Stderr, "Producer")
	fmt.Fprintln(os.Stderr, "--------")
	if producer != nil {
		fmt.Fprintln(os.Stderr, "- exited? NO")
		fmt.Fprintf(os.Stderr, "- goroutine finished? NO (blocked on %s)\n", producer.Blocking)
	} else {
		fmt.Fprintln(os.Stderr, "- exited? YES")
		fmt.Fprintln(os.Stderr, "- goroutine finished? YES")
	}
	fmt.Fprintln(os.Stderr, "")

	fmt.Fprintln(os.Stderr, "Application")
	fmt.Fprintln(os.Stderr, "-----------")
	fmt.Fprintf(os.Stderr, "- target contexts cancelled? %v\n", scanCtxErr != nil)
	fmt.Fprintf(os.Stderr, "- drainCtx cancelled? %v\n", drainCtxErr != nil)
	fmt.Fprintln(os.Stderr, "")

	fmt.Fprintln(os.Stderr, "Blocked Goroutines")
	fmt.Fprintln(os.Stderr, "------------------")
	for _, gr := range routines {
		if gr.Blocking != "" && !strings.Contains(gr.Blocking, "IO wait") && gr.TopFunc != "runtime.gopark" {
			if containsFunc(gr.Stack, "testing.") || containsFunc(gr.Stack, "diagnostics.RunDiagnostics") {
				continue // skip test and self
			}
			fmt.Fprintf(os.Stderr, "- goroutine %s\n", gr.ID)
			fmt.Fprintf(os.Stderr, "  package:  %s\n", extractPackage(gr.TopFunc))
			fmt.Fprintf(os.Stderr, "  file:     %s\n", gr.File)
			fmt.Fprintf(os.Stderr, "  line:     %s\n", gr.Line)
			fmt.Fprintf(os.Stderr, "  blocking: %s\n", gr.Blocking)
			fmt.Fprintln(os.Stderr, "")
		}
	}

	fmt.Fprintln(os.Stderr, "Dependency Chain Analysis")
	fmt.Fprintln(os.Stderr, "-------------------------")

	if mainRoutine != nil {
		fmt.Fprintln(os.Stderr, "Main goroutine")
		fmt.Fprintln(os.Stderr, "    ↓")
		if mainRoutine.Blocking == "channel receive" {
			fmt.Fprintf(os.Stderr, "waiting for channel in %s\n", mainRoutine.TopFunc)
			fmt.Fprintln(os.Stderr, "    ↓")
			if recManager != nil {
				fmt.Fprintln(os.Stderr, "Recursion Manager")
				fmt.Fprintln(os.Stderr, "    ↓")
				fmt.Fprintf(os.Stderr, "blocked on %s in %s\n", recManager.Blocking, recManager.TopFunc)
			} else if len(waitGroups) > 0 {
				fmt.Fprintln(os.Stderr, "WaitGroup")
				fmt.Fprintln(os.Stderr, "    ↓")
				fmt.Fprintf(os.Stderr, "waiting for workers (%d active)\n", activeWorkers)
			}
		} else {
			fmt.Fprintf(os.Stderr, "blocked on %s\n", mainRoutine.Blocking)
		}
	}
	fmt.Fprintln(os.Stderr, "\n=== END DIAGNOSTIC ===")
}

func containsFunc(stack []string, target string) bool {
	for _, line := range stack {
		if strings.Contains(line, target) {
			return true
		}
	}
	return false
}

func extractPackage(funcName string) string {
	idx := strings.LastIndex(funcName, ".")
	if idx == -1 {
		return funcName
	}
	return funcName[:idx]
}
