package main

import (
  "fmt"
  "math"
  "os"
  "os/exec"
  "runtime"
  "strconv"
  "strings"
  "sync"
)

func SplitCommand(cmd string) (string, []string) {
  fs := strings.Fields(cmd)
  
  return fs[0], fs[1:]
}

func PrintCommand(root string, cacheDir string, cmdName string, args []string) {
  var b strings.Builder
  b.WriteString(cmdName)

  cacheFileCount := 0
  wasPchFile := false

  printCacheFileInfo := func() {
    b.WriteString(" ")
    if wasPchFile {
      b.WriteString("<pch>")
    } else if cacheFileCount == 1 {
      b.WriteString("<obj>")
    } else {
      b.WriteString("<")
      b.WriteString(strconv.Itoa(cacheFileCount))
      b.WriteString("-objs>")
    }
    cacheFileCount = 0
    wasPchFile = false
  }

  for _, arg := range args {
    if strings.HasPrefix(arg, cacheDir) {
      cacheFileCount += 1
    } else {
      if cacheFileCount != 0 {
        printCacheFileInfo()
      }

      b.WriteString(" ")
      if strings.HasPrefix(arg, root) {
        b.WriteString(".")
        b.WriteString(strings.TrimPrefix(arg, root))
      } else {
        b.WriteString(arg)
      }
    }
  }

  if cacheFileCount != 0 {
    printCacheFileInfo()
  }

  fmt.Println(b.String())
}

func RunCommand(cmdName string, args []string) error {
  cmd := exec.Command(cmdName, args...)

  cmd.Stdout = os.Stdout
  cmd.Stdin = os.Stdin
  cmd.Stderr = os.Stderr

  return cmd.Run()
}

func RunPar(n int, fn func(i int) error) error {
  nProc := runtime.NumCPU()

  if nProc > n {
    nProc = n
  }

  nGroups := int(math.Ceil(float64(n)/float64(nProc)))

  errs := make([]error, n)

  for iGroup := 0; iGroup < nGroups; iGroup++ {
    groupSize := nProc
    if iGroup == nGroups - 1 {
      groupSize = n - iGroup*nProc
    }

    var wg sync.WaitGroup
    wg.Add(groupSize)

    for iThread := 0; iThread < groupSize; iThread++ {
      go func(i int) {
        errs[i] = fn(i)

        wg.Done()
      }(iThread + iGroup*nProc)
    }

    wg.Wait()

    for _, err := range errs {
      if err != nil {
        return err
      }
    }

  }

  return nil
}
