package main

import (
  "encoding/json"
  "errors"
  "io/ioutil"
  "os"
  "path/filepath"
  "time"
)

type Config struct {
  Lang     string `json:"lang"`
  Dst      string `json:"dst"`
  Compiler string `json:"compiler"`
  ExeFlags string `json:"exe-flags"`
  LibFlags string `json:"lib-flags"`
}

var (
  CONFIG_MOD_TIME = time.Time{}
)

func main_init(root string) error {
  cfgDst := filepath.Join(root, BUILD_CONFIG)
  if _, err := os.Stat(cfgDst); !os.IsNotExist(err) {
    return errors.New(cfgDst + " already exists")
  }

  // TODO: get input from user
  config := Config{
    Lang: "cpp",
    Dst: "./build",
    Compiler: "clang-11",
    ExeFlags: "-Wall -std=c++2a -lstdc++ -lm",
    LibFlags: "",
  }

  b, err := json.MarshalIndent(config, "", "  ")
  if err != nil {
    return err
  }

  return ioutil.WriteFile(cfgDst, b, 0644)
}

func ReadConfig(root string) (*Config, error) {
  cfg := &Config{}

  cfgPath := filepath.Join(root, BUILD_CONFIG)
  stat, err := os.Stat(cfgPath)
  if err != nil {
    if os.IsNotExist(err) {
      return nil, errors.New(cfgPath + " not found")
    }

    return nil, err
  } else if stat.IsDir() {
    return nil, errors.New(cfgPath + " is dir")
  }

  CONFIG_MOD_TIME = stat.ModTime()

  b, err := ioutil.ReadFile(cfgPath)
  if err != nil {
    return nil, err
  }

  if err := json.Unmarshal(b, &cfg); err != nil {
    return nil, err
  }

  return cfg, err
}
