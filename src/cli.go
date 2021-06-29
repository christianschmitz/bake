package main

import (
  "errors"
  "os"
  "path/filepath"
  "sort"
  "strings"
)

func FindString(keys[]string, key string) int {
  for i, check := range keys {
    if check == key {
      return i
    }
  }

  return -1
}

func ContainsString(keys []string, key string) bool {
  return FindString(keys, key) > -1
}

func ContainsAnyString(keys []string, others []string) bool {
  sort.Strings(keys)
  sort.Strings(others)

  j := 0
  i := 0
  for (i < len(keys)) && (j < len(others)) {
    key := keys[i]
    other := others[j]
    if other < key {
      j++
    } else if other > key {
      i++
    } else {
      return true
    }
  }

  return false
}

func SortUnique(strs []string) []string {
  sort.Strings(strs)

  uniq := make([]string, 0)
  for i := 0; i < len(strs); i++ {
    if i == 0 || strs[i] != strs[i-1] {
      uniq = append(uniq, strs[i])
    }
  }

  return uniq
}

func ContainsHelp(args []string) bool {
  for _, arg := range args {
    if arg == "-?" || arg == "-h" || arg == "--help" {
      return true
    }
  }

  return false
}

func ParseBoolFlags(args []string, flagNames []string, result []*bool) []string {
  remaining  := make([]string, 0)

  for _, arg := range args {
    if strings.HasPrefix(arg, "-") {
      if i := FindString(flagNames, arg); i > -1 {
        *(result[i]) = true
      } else {
        remaining = append(remaining, arg)
      }
    } else {
      remaining = append(remaining, arg)
    }
  }

  return remaining
}

func ParseStringFlags(args []string, flagNames []string, result []*string) ([]string, error) {
  remaining  := make([]string, 0)

  for i := 0; i < len(args); i++ {
    arg := args[i]

    if strings.HasPrefix(arg, "-") {
      if j := FindString(flagNames, arg); j > -1 {
        if i >= len(args) {
          return nil, errors.New(arg + " expects an argument")
        }

        val := args[i+1]
        i += 1

        *(result[j]) = val
      } else {
        remaining = append(remaining, arg)
      }
    } else {
      remaining = append(remaining, arg)
    }
  }

  return remaining, nil
}

func AssertNoArgs(args []string) error {
  if len(args) != 0 {
    return errors.New("unexpected arg " + args[0])
  }

  return nil
}

func ParseGeneralArgs(args []string, force *bool, dryRun *bool, dir *string) ([]string, error) {
  args = ParseBoolFlags(args, []string{"-f", "-B", "-n"}, []*bool{force, force, dryRun})

  var err error

  args, err = ParseStringFlags(args, []string{"-C"}, []*string{dir})
  if err != nil {
    return nil, err
  }

  if *dir != "" {
    *dir, err = filepath.Abs(*dir)
    if err != nil {
      return nil, err
    }
  }

  return args, nil
}

func ParseGeneralArgsFindMakefile(args []string, force *bool, dryRun *bool, dir *string) ([]string, error) {
  rem, err := ParseGeneralArgs(args, force, dryRun, dir)
  if err != nil {
    return nil, err
  }

  if *dir == "" {
    *dir, err = FindMakefileDir()
    if err != nil {
      return nil, err
    }
  }

  return rem, nil
}

func ParseGeneralArgsDefaultDirPwd(args []string, force *bool, dryRun *bool, dir *string) ([]string, error) {
  rem, err := ParseGeneralArgs(args, force, dryRun, dir)
  if err != nil {
    return nil, err
  }

  if *dir == "" {
    var err error
    *dir, err = os.Getwd()
    if err != nil {
      return nil, err
    }
  }

  return rem, nil
}

