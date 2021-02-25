package main

import (
	"context"

	//快速构建命令行应用程序
	"github.com/urfave/cli/v2"
	//Trace包 调试跟踪信息
	"go.opencensus.io/trace"

	//build全局信息和一些基础定义（编译）
	"github.com/filecoin-project/lotus/build"
	//cmd包，包括基础cmd和开发者cmd
	lcli "github.com/filecoin-project/lotus/cli"
	//log 日志信息
	"github.com/filecoin-project/lotus/lib/lotuslog"
	//Trace包 调试跟踪信息
	"github.com/filecoin-project/lotus/lib/tracing"
	//repo包关于repo信息的存放，包括API，
	//fsAPI           = "api"			API对外接口，example：/ip4/172.17.10.11/tcp/13245/http
	//fsAPIToken      = "token"			Token密码，外部访问需要提供该密码
	//fsConfig        = "config.toml"	config.toml，默认配置，一般设置API对外接口，Fee数量，Storage，TasksLimit
	//fsStorageConfig = "storage.json"  落盘位置
	//fsDatastore     = "datastore"		放置client metadata staging
	//fsLock          = "repo.lock"		repo文件锁
	//fsKeystore      = "keystore"		秘钥存放
	"github.com/filecoin-project/lotus/node/repo"
)

var AdvanceBlockCmd *cli.Command

func main() {
	//节点类型，Lotus Miner Worker节点
	build.RunningNodeType = build.NodeFull

	//日志等级
	lotuslog.SetupLogLevels()

	//初始化cmd基础模块
	local := []*cli.Command{
		//启动daemon程序
		DaemonCmd,
		//备份功能
		backupCmd,
	}
	//开发模式Cmd
	if AdvanceBlockCmd != nil {
		local = append(local, AdvanceBlockCmd)
	}

	//Debug工具 设置Url，用于接收Traces
	jaeger := tracing.SetupJaegerTracing("lotus")
	defer func() {
		if jaeger != nil {
			jaeger.Flush()
		}
	}()

	//BeforeFunc cmd
	for _, cmd := range local {
		cmd := cmd
		originBefore := cmd.Before
		cmd.Before = func(cctx *cli.Context) error {
			//注册Traces
			trace.UnregisterExporter(jaeger)
			jaeger = tracing.SetupJaegerTracing("lotus/" + cmd.Name)

			if originBefore != nil {
				return originBefore(cctx)
			}
			return nil
		}
	}
	//创建一个Trace的Context
	ctx, span := trace.StartSpan(context.Background(), "/cli")
	defer span.End()

	app := &cli.App{
		Name:                 "lotus",
		Usage:                "Filecoin decentralized storage network client",
		Version:              build.UserVersion(),
		//允许bash 命令
		EnableBashCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "repo",
				EnvVars: []string{"LOTUS_PATH"},
				Hidden:  true,
				Value:   "~/.lotus", // TODO: Consider XDG_DATA_HOME
			},
		},

		//WithCategory("basic", sendCmd),
		//WithCategory("basic", walletCmd),
		//WithCategory("basic", clientCmd),
		//WithCategory("basic", multisigCmd),
		//WithCategory("basic", paychCmd),
		//WithCategory("developer", authCmd), lotus-miner auth api-info --perm admin
		//WithCategory("developer", mpoolCmd),
		//WithCategory("developer", stateCmd),
		//WithCategory("developer", chainCmd),
		//WithCategory("developer", logCmd),
		//WithCategory("developer", waitApiCmd),
		//WithCategory("developer", fetchParamCmd),
		//WithCategory("network", netCmd),
		//WithCategory("network", syncCmd),
		//pprofCmd,
		//VersionCmd,
		Commands: append(local, lcli.Commands...),
	}
	app.Setup()
	app.Metadata["traceContext"] = ctx
	app.Metadata["repoType"] = repo.FullNode

	//入口
	lcli.RunApp(app)
}
