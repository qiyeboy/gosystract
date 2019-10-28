package main

import (
	"bufio"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/golang-collections/collections/stack"
	"github.com/pjbgf/dump/pkg"
	"github.com/sirupsen/logrus"
)

var (
	dumpFileName  string
	logger        *logrus.Logger
	preValidation chan string

	processedNames map[string]bool
	namesMutex     sync.RWMutex
	processedIDs   map[uint8]bool
	idsMutex       sync.RWMutex

	symbols map[string]symbolDefinition
)

type symbolDefinition struct {
	syscallIDs []uint8
	subCalls   []string
}

const (
	callCaptureRegex         string = ".+CALL.(\b([a-zA-Z0-9_.\\/]|\\.|\\(\\*[a-z]+\\))+\b)+"
	callsFromEntryPointRegex string = "main.go" + callCaptureRegex
)

func init() {
	logger = logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	processedNames, processedIDs = make(map[string]bool), make(map[uint8]bool)
	symbols = make(map[string]symbolDefinition)
}

func main() {

	consume := func(id uint8) {
		logger.Infof("syscall found: %d", id)
	}

	parseFile()

	symbolCalls := []string{"main.init.0", "main.init.1", "main.main"}
	for _, symbol := range symbolCalls {
		processSymbol(symbol, consume)
	}

	// readFromFile()
}

func processSymbol(symbol string, consume func(uint8)) {
	namesMutex.RLock()
	_, exists := processedNames[symbol]
	namesMutex.RUnlock()

	if !exists {
		namesMutex.Lock()
		processedNames[symbol] = true
		namesMutex.Unlock()

		logger.Debugln(symbol)
		if s, found := symbols[symbol]; found {

			for _, id := range s.syscallIDs {
				consume(id)
			}

			for _, name := range s.subCalls {

				processSymbol(name, consume)
			}
		}
	}
}

func parseFile() {

	dumpFileName = "caps-dump"
	file, err := os.Open(dumpFileName)
	if err != nil {
		panic(file)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()

		symbolName, found := pkg.GetSymbolName(line)
		stack := stack.New()
		symbol := symbolDefinition{
			subCalls:   make([]string, 0),
			syscallIDs: make([]uint8, 0),
		}

		for found {
			if scanner.Scan() {
				line = scanner.Text()

				if pkg.IsEndOfSymbol(line) {
					break
				}

				if subcall, found := pkg.GetCallTarget(line); found {
					symbol.subCalls = append(symbol.subCalls, subcall)
					continue
				}

				if id, found := pkg.TryGetSyscallID(line, stack); found {
					symbol.syscallIDs = append(symbol.syscallIDs, id)
					continue
				}

				pkg.StackSyscallIDIfNecessary(line, stack)
			} else {
				break
			}
		}

		if len(symbol.subCalls) > 0 || len(symbol.syscallIDs) > 0 {
			symbols[symbolName] = symbol
			logger.Debugln(symbolName)
			logger.Debugln(symbol)
		} else {
			logger.Debugf("[%s] skipping empty symbol", symbolName)
		}
	}
}

func readFromFile() {
	dumpFileName = "caps-dump"

	symbolCalls := []string{"main.init.0", "main.init.1", "main.main"}

	consume := func(id uint8) {
		logger.Infof("syscall found: %d", id)
	}

	for _, s := range symbolCalls {
		processPerStage(s, consume)
	}
}

func getMatchingLines(regex string) []string {
	file, err := os.Open(dumpFileName)
	if err != nil {
		panic(file)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)

	lines := make([]string, 0)
	for scanner.Scan() {

		line := scanner.Text()
		re := regexp.MustCompile(regex)
		captures := re.FindStringSubmatch(line)

		if captures != nil && len(captures) > 0 {
			lines = append(lines, captures[1])
		}
	}
	return lines
}

func getSubCalls(symbolName string) []string {
	file, err := os.Open(dumpFileName)
	if err != nil {
		panic(file)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)

	subCalls := make([]string, 0)
	processedSymbols := make(map[string]bool)

	for scanner.Scan() {
		line := scanner.Text()
		if pkg.IsSymbolDefinition2(line, symbolName) {
			logger.Debugf("-- [SUBCALLS] %s\n", symbolName)
			for scanner.Scan() {
				line = scanner.Text()
				if line == "" || line == "\n" || strings.Contains(line, "TEXT ") {
					return subCalls
				}

				target, found := pkg.GetCallTarget(line)
				if found {

					if _, exists := processedSymbols[target]; !exists {
						processedSymbols[target] = true
						subCalls = append(subCalls, target)
					}
				}
			}
		}
	}
	return subCalls
}

func getSyscalls(symbolName string) []uint8 {
	file, err := os.Open(dumpFileName)
	if err != nil {
		panic(file)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)

	sysCallIDs := make([]uint8, 0)
	for scanner.Scan() {
		line := scanner.Text()
		if pkg.IsSymbolDefinition2(line, symbolName) {
			logger.Debugf("-- [SYSCALLS] %s\n", symbolName)
			stack := stack.New()
			for scanner.Scan() {
				line = scanner.Text()
				if line == "" || line == "\n" || strings.Contains(line, "TEXT ") {
					return sysCallIDs
				}

				if id, found := pkg.GetSyscallID(line); found {
					stack.Push(id)
				}

				if pkg.ContainsSyscall(line) {
					val1 := stack.Pop()
					val2 := stack.Pop()

					syscallID := val1
					if val2 != nil {
						syscallID = val2
					}
					sysCallIDs = append(sysCallIDs, syscallID.(uint8))
				}
			}
		}
	}
	return sysCallIDs
}

func processPerStage(symbolName string, consume func(uint8)) {
	var (
		symbolNames, sysCallIDs = make(chan string), make(chan uint8)
		done                    = make(chan struct{})
	)
	go func() {
		defer close(symbolNames)
		names := getSubCalls(symbolName)
		for _, n := range names {
			namesMutex.RLock()
			_, exists := processedNames[n]
			namesMutex.RUnlock()

			if !exists {
				namesMutex.Lock()
				processedNames[n] = true
				namesMutex.Unlock()

				symbolNames <- n
				processPerStage(n, consume)
			}
		}
	}()

	go func() {
		defer close(sysCallIDs)

		for n := range symbolNames {
			ids := getSyscalls(n)
			for _, id := range ids {
				idsMutex.RLock()
				_, exists := processedIDs[id]
				idsMutex.RUnlock()

				if !exists {
					idsMutex.Lock()
					processedIDs[id] = true
					idsMutex.Unlock()

					sysCallIDs <- id
				}
			}
		}
	}()

	go func() {
		for i := range sysCallIDs {
			consume(i)
		}

		close(done)
	}()

	<-done
}
