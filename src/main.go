package main

import (
  "errors"
  "fmt"
  "os"
  "path/filepath"
  "strings"
)

func main() {
  if err := mainInner(); err != nil {
    fmt.Fprintf(os.Stderr, "%s\n", err.Error())
    os.Exit(1)
  }
}

func printUsage() {
  var b strings.Builder

  b.WriteString("bake [MODE | [TARGET]] [OPTIONS]\n")
  b.WriteString("\nModes:\n")
  b.WriteString("  --init            wizard to create new makefile recipe\n")
  b.WriteString("  --project <type>  go or c\n")
  b.WriteString("\nProject mode options:\n")
  b.WriteString("  --compiler <compiler-cmd>\n")
  b.WriteString("  --linker   <linker-cmd>\n")
  b.WriteString("  --pch      <pch-cmd>\n")
  b.WriteString("  --dst      <dst-dir>\n")
  b.WriteString("\nGeneral options:\n")
  b.WriteString("  -f/-B             force\n")
  b.WriteString("  -n                dry-run\n")
  b.WriteString("  -C <dir>          change directory\n")
  b.WriteString("  -h                display this message\n")

  fmt.Fprintf(os.Stderr, "%s", b.String())
}

func mainInner() error {
  args := os.Args[1:]

  if ContainsHelp(args) {
    printUsage()
    return nil
  }

  if len(args) == 0 {
    return mainMake([]string{})
  } else if strings.HasPrefix(args[0], "--") {
    mode := args[0][2:]

    switch mode {
    case "init":
      return mainBakeInit(args[1:])
    case "project":
      return mainBakeProject(args[1:])
    default:
      return errors.New("mode " + mode + " not recognized")
    }
  } else if strings.HasPrefix(args[0], "-") {
    return mainMake(args)
  } else {
    return mainMakeTarget(args[0], args[1:])
  }
}

func mainMake(args []string) error {
  var (
    force  bool
    dryRun bool
    dir    string
  )

  rem, err := ParseGeneralArgsFindMakefile(args, &force, &dryRun, &dir)
  if err != nil {
    return err
  }

  if err := AssertNoArgs(rem); err != nil {
    return err
  }

  cmdArgs, err := SetupMakeArgs(dir, force, dryRun)
  if err != nil {
    return err
  }

  return RunCommand("make", cmdArgs)
}

// TODO: prompt for user input
func buildBakeRecipe() string {
  var b strings.Builder

  b.WriteString("PROJECT_TYPE=\"c\"\n")
  b.WriteString("CPP_DIALECT=\"c++2a\"\n")
  b.WriteString("COMPILER_CMD=\"clang-11 -std=$(CPP_DIALECT) {include} -c {source} -o {output}\"\n")
  b.WriteString("LINKER_CMD=\"clang-11 -std=$(CPP_DIALECT) {libs} -o {output} {objects}\"\n")
  b.WriteString("EMIT_PCH_CMD=\"clang-11 -std=$(CPP_DIALECT) {include} {header} -o {output}\"\n")
  b.WriteString("INCLUDE_PCH_OPTS=\"-include-pch {pch}\"\n")
  b.WriteString("DST_DIR=\"./build/\"\n\n")
  b.WriteString("compile:\n")
  b.WriteString("\t@bake --project $(PROJECT_TYPE) --compiler $(COMPILER_CMD) --linker $(LINKER_CMD) --dst $(DST_DIR) --emit-pch $(EMIT_PCH_CMD) --include-pch $(INCLUDE_PCH_OPTS)")

  return b.String()
}

func mainBakeInit(args []string) error {
  var (
    force  bool
    dryRun bool
    dir    string
  )

  rem, err := ParseGeneralArgsDefaultDirPwd(args, &force, &dryRun, &dir)
  if err != nil {
    return err
  }

  if force && dryRun {
    return errors.New("-f/-B and -n are conflicting flags for bake --init")
  }

  if err := AssertNoArgs(rem); err != nil {
    return err
  }

  exists, err := MakefileExists(dir)
  if err != nil {
    return err
  }

  recipe := buildBakeRecipe()

  if force || (!dryRun && !exists) {
    if err := WriteMakefile(dir, recipe); err != nil {
      return err
    }
  } else {
    fmt.Println(recipe)
  }

  return nil
}

func mainMakeTarget(target string, args []string) error {
  var (
    force  bool
    dryRun bool
    dir    string
  )

  rem, err := ParseGeneralArgsFindMakefile(args, &force, &dryRun, &dir)
  if err != nil {
    return err
  }

  if err := AssertNoArgs(rem); err != nil {
    return err
  }

  isMakefileTarget, err := TargetExists(dir, target)
  if err != nil {
    return err
  }

  cmd := "make"
  cmdArgs, err := SetupMakeArgs(dir, force, dryRun)

  if isMakefileTarget {
    cmdArgs = append(cmdArgs, target)
  } else {
    if err := os.Setenv("BAKE_TARGET", target); err != nil {
      return err
    }
  }

  return RunCommand(cmd, cmdArgs)
}

func mainBakeProject(args []string) error {
  pType := args[0]
  args = args[1:]

  var project Project
  var err error

  switch pType {
  case "c":
    project, err = NewCProject(args)
  //case "go":
    //project, err = NewGoProject(args)
  default:
    return errors.New("unrecognized project type " + pType)
  }

  if err != nil {
    return err
  }

  home := os.Getenv("HOME")
  CACHE_DIR = filepath.Join(home, CACHE_DIR_REL)
  if err := os.MkdirAll(CACHE_DIR, 0755); err != nil {
    return err
  }

  if err := project.ResolveDeps(); err != nil {
    return err
  }

  bakeTarget := os.Getenv("BAKE_TARGET")

  if bakeTarget != "" {
    return project.BuildTarget(bakeTarget)
  } else {
    return project.Build()
  }
}
