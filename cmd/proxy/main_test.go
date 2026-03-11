package main

import (
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootCmdPersistentPreRunE(t *testing.T) {
	originalLog := log
	originalLogLevel := logLevel

	t.Cleanup(func() {
		log = originalLog
		logLevel = originalLogLevel
	})

	log = logrus.New()
	logLevel = "debug"

	require.NoError(t, rootCmd.PersistentPreRunE(rootCmd, nil))
	assert.Equal(t, logrus.DebugLevel, log.GetLevel())

	_, ok := log.Formatter.(*logrus.JSONFormatter)
	assert.True(t, ok)

	logLevel = "not-a-level"
	require.Error(t, rootCmd.PersistentPreRunE(rootCmd, nil))
}

func TestRunServeReturnsConfigError(t *testing.T) {
	originalCfgFile := cfgFile

	t.Cleanup(func() {
		cfgFile = originalCfgFile
	})

	cfgFile = filepath.Join(t.TempDir(), "missing.yaml")

	err := runServe(rootCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading config")
}
