package shutdown

import (
	"go.uber.org/zap"
	"os"
	"os/signal"
	"syscall"
)

func GracefulStop(stop func()) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(
		signalChan,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	)

	<-signalChan
	zap.L().Info("os.Interrupt - shutting down...")

	go func() {
		<-signalChan
		zap.L().Fatal("os.Kill - terminating...")
	}()

	stop()

	os.Exit(0)
}
