//go:build windows

package main

// Mesa software OpenGL self-extraction for VM / no-GPU environments.
//
// Why this exists: Fyne/GLFW links against opengl32.dll at compile time
// (PE import table). Windows resolves that DLL *before* any Go code runs,
// so it always loads the system stub first (OpenGL 1.1 only). Mesa's
// llvmpipe build provides full OpenGL 3.3 in software, which is what
// GLFW needs to create a context.
//
// Strategy: extract Mesa's opengl32.dll next to the exe on first launch,
// then restart the process so Windows finds the app-dir DLL before System32.
// On all subsequent launches the DLL is already present and the restart
// branch is skipped — no overhead.
//
// To supply the DLL: place the Mesa x64 llvmpipe opengl32.dll at
//   cmd/flow-launcher/assets/opengl32.dll
// and recompile. The file is available from:
//   https://github.com/pal1000/mesa-dist-win/releases

import (
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed assets/opengl32.dll
var mesaOpenGLDLL []byte

func init() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	dllPath := filepath.Join(filepath.Dir(exePath), "opengl32.dll")

	if _, err := os.Stat(dllPath); err == nil {
		return // Mesa DLL already present; Windows will use it on this run.
	}

	// First launch: extract and restart so the new process picks up the DLL
	// from the application directory (takes priority over System32).
	tmp := dllPath + ".tmp"
	if err := os.WriteFile(tmp, mesaOpenGLDLL, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, dllPath); err != nil {
		_ = os.Remove(tmp)
		return
	}

	// Re-execute with the same arguments. The child process will find
	// opengl32.dll in the app directory before System32 and use Mesa.
	cmd := exec.Command(exePath, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
	}
	os.Exit(0)
}
