package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NVIDIA/gpu-monitoring-tools/bindings/go/nvml"

	"github.com/mum4k/termdash"
	"github.com/mum4k/termdash/cell"
	"github.com/mum4k/termdash/container"
	"github.com/mum4k/termdash/linestyle"
	"github.com/mum4k/termdash/terminal/termbox"
	"github.com/mum4k/termdash/terminal/terminalapi"
	"github.com/mum4k/termdash/widgets/gauge"
)

type Resources struct {
	Power       uint
	Temperature uint
	GPUUtil     uint
	DecUtil     uint
	EncUtil     uint
	MemoryUtil  uint64
}

var TotalMemory uint64

// playType indicates how to play a gauge.
type playType int

const (
	playTypePercent playType = iota
	playTypeAbsolute
)

// playGauge continuously changes the displayed percent value on the gauge by the
// step once every delay. Exits when the context expires.
func playGauge(ctx context.Context, g *gauge.Gauge, gpuData *Resources, delay time.Duration, pt playType) {
	ticker := time.NewTicker(delay)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			switch pt {
			case playTypePercent:
				if err := g.Percent(int(gpuData.GPUUtil)); err != nil {
					panic(err)
				}
			case playTypeAbsolute:
				if err := g.Absolute(int(gpuData.GPUUtil), 100); err != nil {
					panic(err)
				}
			}

		case <-ctx.Done():
			return
		}
	}
}

func updateValues(ctx *Resources, devices []*nvml.Device, count uint, delay time.Duration) {
	ticker := time.NewTicker(delay)
	defer ticker.Stop()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

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

			*ctx = totalUse
			/*
				fmt.Printf("Total VRAM: %5d MB\n", TotalMemory)
				fmt.Printf("Total Power Usage: %5dW\n", totalUse.Power)
				fmt.Printf("Avg Temp: %5d°\n", totalUse.Temperature)
				fmt.Printf("Avg GPU: %5d%%\n", totalUse.GPUUtil)
				fmt.Printf("Avg Decoder: %5d%%\n", totalUse.DecUtil)
				fmt.Printf("Avg Encoder: %5d%%\n", totalUse.EncUtil)
				fmt.Printf(" VRAM Usage: %5d MB\n", totalUse.MemoryUtil)
			*/
		case <-sigs:
			return
		}
	}
}

func main() {
	nvml.Init()
	defer nvml.Shutdown()

	t, err := termbox.New()
	if err != nil {
		panic(err)
	}
	defer t.Close()

	delayTime := 1 * time.Second

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

	for _, device := range devices {
		mem := device.Memory
		TotalMemory += *mem
	}

	var totalGPUValues = Resources{}

	go updateValues(&totalGPUValues, devices, count, delayTime)

	ctx, cancel := context.WithCancel(context.Background())
	withLabel, err := gauge.New(
		gauge.Height(3),
		gauge.TextLabel("GPU Usage"),
		gauge.Color(cell.ColorRed),
		gauge.FilledTextColor(cell.ColorBlack),
		gauge.EmptyTextColor(cell.ColorYellow),
	)
	if err != nil {
		panic(err)
	}
	go playGauge(ctx, withLabel, &totalGPUValues, delayTime, playTypePercent)

	c, err := container.New(
		t,
		container.SplitVertical(
			container.Left(
				container.Border(linestyle.Light),
				container.BorderTitle("PRESS Q TO QUIT"),
			),
			container.Right(
				container.PlaceWidget(withLabel),
			),
		),
	)

	if err != nil {
		panic(err)
	}

	quitter := func(k *terminalapi.Keyboard) {
		if k.Key == 'q' || k.Key == 'Q' {
			cancel()
		}
	}

	if err := termdash.Run(ctx, t, c, termdash.KeyboardSubscriber(quitter)); err != nil {
		panic(err)
	}
}
