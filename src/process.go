package main

import (
	"fmt"
	"os/exec"
)

func (g *HeistGUI) executeKill() {
	cmd := exec.Command("taskkill", "/F", "/IM", "GTA5_Enhanced.exe")
	if err := cmd.Run(); err != nil {
		g.appendLog(fmt.Sprintf("[X] Failed to kill process: %v", err))
	} else {
		g.appendLog("[✓] GTA5 process terminated instantly!")
	}
}
