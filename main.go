package main

import (
	"github.com/mzj-xk/cart/common"
	"github.com/mzj-xk/cart/domain/repository"
	"github.com/mzj-xk/cart/handler"
	pb "github.com/mzj-xk/cart/proto"

	service2 "github.com/mzj-xk/cart/domain/service"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"github.com/micro/go-micro/v2"
	"github.com/micro/go-micro/v2/registry"
	"github.com/micro/go-micro/v2/util/log"
	consul2 "github.com/micro/go-plugins/registry/consul/v2"
	ratelimit "github.com/micro/go-plugins/wrapper/ratelimiter/uber/v2"
	opentracing2 "github.com/micro/go-plugins/wrapper/trace/opentracing/v2"
	"github.com/micro/micro/v3/service/logger"
	"github.com/opentracing/opentracing-go"
)

// 每秒连接数
var QPS = 100

func main() {
	// 配置中心
	consulConfig, err := common.GetConsulConfig("127.0.01", 8500, "/micro/config")
	if err != nil {
		log.Error(err)
	}

	consul := consul2.NewRegistry(func(options *registry.Options) {
		options.Addrs = []string{
			"127.0.0.1:8500",
		}
	})

	// 链路追踪
	t, io, err := common.NewTracer("go.micro.service.cart", "localhost:6831")
	if err != nil {
		log.Error(err)
	}

	defer io.Close()
	opentracing.SetGlobalTracer(t)

	// 数据库连接
	mysqlInfo := common.GetMysqlFromConsul(consulConfig, "mysql")
	// 创建数据库连接
	db, err := gorm.Open("mysql", mysqlInfo.User+":"+mysqlInfo.Pwd+"@tcp("+mysqlInfo.Host+")/"+mysqlInfo.Database+"?charset=utf8")
	if err != nil {
		log.Error(err)
	}
	defer db.Close()
	db.SingularTable(true)

	err = repository.NewCartRepository(db).InitTable()
	if err != nil {
		log.Error(err)
	}

	// Create service
	service := micro.NewService(
		micro.Name("go.micro.service.cart"),
		micro.Version("latest"),
		//暴露的服务地址
		micro.Address("0.0.0.0:8087"),
		//注册中心
		micro.Registry(consul),
		//链路追踪
		micro.WrapHandler(opentracing2.NewHandlerWrapper(opentracing.GlobalTracer())),
		//添加限流
		micro.WrapHandler(ratelimit.NewHandlerWrapper(QPS)),
	)
	service.Init()
	cartDataService := service2.NewCartDataService(repository.NewCartRepository(db))

	// Register handler
	pb.RegisterCartHandler(service.Server(), &handler.Cart{CartDataService: cartDataService})

	// Run service
	if err := service.Run(); err != nil {
		logger.Fatal(err)
	}
}
