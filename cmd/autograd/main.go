package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/PrairieLearn/autograd/config"
	"github.com/PrairieLearn/autograd/consumer"
	"github.com/PrairieLearn/autograd/grader"
	graderconfig "github.com/PrairieLearn/autograd/grader/config"
	"github.com/PrairieLearn/autograd/repo"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func main() {
	autogradRoot, err := config.GetAutogradRoot()
	if err != nil {
		log.Fatalf("Failed to get autograd root: %s", err)
	}

	log.Printf("Starting autograd agent at %s", autogradRoot)

	cfg, err := config.Load(autogradRoot)
	if err != nil {
		log.Fatalf("Failed to load autograd config: %s", err)
	}

	err = repo.Sync(
		cfg.GraderRepo.RepoURL,
		cfg.GraderRepo.Commit,
		autogradRoot,
		cfg.GraderRepo.Credentials.PublicKey,
		cfg.GraderRepo.Credentials.PrivateKey,
		cfg.GraderRepo.Credentials.Passphrase)
	if err != nil {
		log.Fatalf("Failed to sync grader repo: %s", err)
	}

	graderCfg, err := graderconfig.Load(grader.GetGraderRoot(autogradRoot))
	if err != nil {
		log.Fatalf("Failed to load grader config: %s", err)
	}

	grader := grader.New(
		autogradRoot,
		graderCfg.Grader.SetupCommands,
		graderCfg.Grader.GradeCommand,
		graderCfg.Grader.CleanupCommands,
		graderCfg.Grader.GradeTimeout)

	sigterm := make(chan os.Signal)
	signal.Notify(sigterm, syscall.SIGTERM)

	isRunning := true
	for isRunning {
		c, err := consumer.NewConsumer(cfg.AMQP.URL, cfg.AMQP.GradingQueue, grader)
		if err != nil {
			log.Warnf("Error initializing AMQP consumer: %s", err)
			time.Sleep(1 * time.Second)
			continue
		}

		log.Info("Listening for grading jobs")

		select {
		case err := <-c.NotifyClose():
			log.Warnf("Closing consumer: %s", err)
		case <-sigterm:
			log.Info("Received SIGTERM, finishing last job")
			isRunning = false
		}

		log.Infof("Shutting down AMQP connection")

		if err := c.Shutdown(); err != nil {
			log.Warnf("Error during shutdown: %s", err)
		}
	}
}
