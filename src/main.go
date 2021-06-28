package main

import (
  "errors"
  "fmt"
  "math"
  "os"
  "path/filepath"
  "runtime"
  "sync"

  "github.com/urfave/cli"
)

const (
  BUILD_CONFIG = "package.json"

  REL_CACHE_DIR = "/.cache/build"
)

var (
  VERSION = ""

  ROOT_PATH = ""

  CACHE_DIR = ""

  INIT = false

  MUTEX = &sync.RWMutex{}
  UPDATED_OBJS = make(map[string]string)
  UP_TO_DATE = true

  FORCE = false
)

func main() {
  app := &cli.App{
    Name: "build",
    Version: VERSION,
    Usage: "automatic compilation of cpp projects, without pesky configuration",
    Flags: []cli.Flag{
      cli.StringFlag{
        Name: "d",
        Destination: &ROOT_PATH,
        Value: ROOT_PATH,
      },
      cli.BoolFlag{
        Name: "init",
        Destination: &INIT,
      },
      cli.BoolFlag{
        Name: "f",
        Destination: &FORCE,
      },
    },
    Action: main_internal,
  }

  app.RunAndExitOnError()
}

func CacheDir() string {
  if CACHE_DIR == "" {

    home := os.Getenv("HOME")
    CACHE_DIR = filepath.Join(home, REL_CACHE_DIR)
    if err := os.MkdirAll(CACHE_DIR, 0755); err != nil {
      panic(err)
    }
  }

  return CACHE_DIR
}

func main_internal(c *cli.Context) error {
  var (
    err       error
    rootGiven bool
  )

  if ROOT_PATH == "" {
    ROOT_PATH, err = os.Getwd()
    if err != nil {
      return err
    }

    rootGiven = false
  } else {
    ROOT_PATH, err = filepath.Abs(ROOT_PATH)
    if err != nil {
      return err
    }

    stat, err := os.Stat(ROOT_PATH); 
    if os.IsNotExist(err) {
      return errors.New("dir " + ROOT_PATH + " not found")
    } else if err != nil {
      return err
    } else if !stat.IsDir() {
      return errors.New(ROOT_PATH + " is not a directory")
    }

    rootGiven = true
  }

  if INIT {
    if len(c.Args()) != 0 {
      return errors.New("unexpected args for -init")
    }

    return main_init(ROOT_PATH)
  } else {
    if !rootGiven {
      curDir := ROOT_PATH
      found := false
      for len(curDir) > 1 {
        fname := filepath.Join(curDir, BUILD_CONFIG)

        stat, err := os.Stat(fname)
        if err == nil {
          if stat.IsDir() {
            return errors.New(fname + " is directory")
          }
          found = true
          break
        }

        curDir = filepath.Dir(curDir)
      }

      if !found {
        return errors.New(BUILD_CONFIG + " not found")
      } else {
        ROOT_PATH = curDir
      }
    } else {
      fname := filepath.Join(ROOT_PATH, BUILD_CONFIG)
      if stat, err := os.Stat(fname); err != nil {
        return err
      } else if stat.IsDir() {
        return errors.New(fname + " is a directory")
      }
    }

    return main_build(ROOT_PATH, c.Args())
  }
}

func main_build(path string, args []string) error {
  if len(args) != 0 {
    return errors.New("target spec not yet supported")
  }

  cfg, err := ReadConfig(path)
  if err != nil {
    return err
  }

  // now read all the files into a data structure
  repo, err := ReadDirsAndFiles(path)
  if err != nil {
    return err
  }

  if err := repo.ReadDeps(repo); err != nil {
    return err
  }

  if err := compileObjs(cfg, repo); err != nil {
    os.Exit(1)
    //return err
  }

  if err := compileExes(path, cfg, repo); err != nil {
    return err
  }

  if UP_TO_DATE {
    fmt.Println("up-to-date")
  }

  return nil
}

func compileObjs(cfg *Config, repo *Dir) error {
  cppFiles := make([]*File, 0)
  if err := repo.WalkFiles(func (f *File) error {
    if f.IsCPP() {
      cppFiles = append(cppFiles, f)
    }

    return nil
  }); err != nil {
    return err
  }

  dirtyFiles := make([]*File, 0)
  for _, cppFile := range cppFiles {
    if !cppFile.ObjUpToDate() {
      dirtyFiles = append(dirtyFiles, cppFile)
    }
  }

  return RunPar(len(dirtyFiles), func(i int) error {
    return dirtyFiles[i].Compile(cfg)
  });
}

func IsUpdatedObj(obj string) bool {
  MUTEX.RLock()

  _, ok := UPDATED_OBJS[obj]

  MUTEX.RUnlock()

  return ok
}

func SetUpdatedObj(obj string) {
  MUTEX.Lock()

  UPDATED_OBJS[obj] = obj

  UP_TO_DATE = false

  MUTEX.Unlock()
}

func SetUpdatedExe(path string) {
  MUTEX.Lock()

  UP_TO_DATE = false

  MUTEX.Unlock()
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

func compileExes(root string, cfg *Config, repo *Dir) error {
  // create the exes
  exeFiles := make([]*File, 0)
  if err := repo.WalkFiles(func (f *File) error {
    if f.ft == EXE_ENTRY {
      exeFiles = append(exeFiles, f)
    }

    return nil
  }); err != nil {
    return err
  }

  dstDir := cfg.Dst 
  if !filepath.IsAbs(dstDir) {
    dstDir = filepath.Join(root, cfg.Dst)
  }

  if err := os.MkdirAll(dstDir, 0755); err != nil {
    return err
  }

  dirtyFiles := make([]*File, 0)

  for _, exeFile := range exeFiles {
    if !exeFile.ExeUpToDate(dstDir, repo) {
      dirtyFiles = append(dirtyFiles, exeFile)
    }
  }

  return RunPar(len(dirtyFiles), func(i int) error {
    exeFile := dirtyFiles[i]

    return exeFile.LinkExe(cfg, dstDir, repo)
  })
}
