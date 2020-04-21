package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NVIDIA/gpu-monitoring-tools/bindings/go/nvml"
)

type Resources struct {
	Power       uint
	Temperature uint
	GPUUtil     uint
	DecUtil     uint
	EncUtil     uint
	MemoryUtil  uint64
}

func main() {
	nvml.Init()
	defer nvml.Shutdown()

	count, err := nvml.GetDeviceCount()
	if err != nil {
		log.Panicln("Error getting device count:", err)
	}

	var devices []*nvml.Device
	for i := uint(0); i < count; i++ {
		device, err := nvml.NewDevice(i)
		if err != nil {
			log.Panicf("Error getting device %d: %v\n", i, err)
		}
		devices = append(devices, device)
	}

	var TotalMemory uint64
	for _, device := range devices {
		mem := device.Memory
		TotalMemory += *mem
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var totalUse = Resources{}

			for i, device := range devices {
				st, err := device.Status()
				if err != nil {
					log.Panicf("Error getting device %d status: %v\n", i, err)
				}

				totalUse.Power += *st.Power
				totalUse.Temperature += *st.Temperature
				totalUse.GPUUtil += *st.Utilization.GPU
				totalUse.DecUtil += *st.Utilization.Decoder
				totalUse.EncUtil += *st.Utilization.Encoder
				totalUse.MemoryUtil += *st.Memory.Global.Used

			}
			totalUse.Temperature /= count
			totalUse.GPUUtil /= count
			totalUse.DecUtil /= count
			totalUse.EncUtil /= count

			fmt.Printf("Total VRAM: %5d MB\n", TotalMemory)
			fmt.Printf("Total Power Usage: %5dW\n", totalUse.Power)
			fmt.Printf("Avg Temp: %5d°\n", totalUse.Temperature)
			fmt.Printf("Avg GPU: %5d%%\n", totalUse.GPUUtil)
			fmt.Printf("Avg Decoder: %5d%%\n", totalUse.DecUtil)
			fmt.Printf("Avg Encoder: %5d%%\n", totalUse.EncUtil)
			fmt.Printf(" VRAM Usage: %5d MB\n", totalUse.MemoryUtil)

		case <-sigs:
			return
		}
	}
}
