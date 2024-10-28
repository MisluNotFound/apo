package database

import (
	"fmt"
	"github.com/CloudDetail/apo/backend/pkg/logger"
	uuid2 "github.com/google/uuid"
	"testing"
)

func Test(t *testing.T) {
	zapLog := logger.NewLogger(logger.WithLevel("debug"))
	repo, err := New(zapLog)
	if err != nil {
		panic(err)
	}

	config := DingTalkConfig{
		AlertName: "group_test",
		UUID:      uuid2.New().String(),
		URL:       "https://oapi.dingtalk.com/robot/send?access_token=4d2bc63b78c61a9c9ba3a09b4c7e96cde36dcecc4ca0f423ef61dae03b172dbe",
		Secret:    "SEC4bdd3a8b5b0c69127bdc04c991d10347be7ff3a986637f7fea8bf31df55c61d3",
	}
	err = repo.CreateDingTalkReceiver(&config)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(config)
}
