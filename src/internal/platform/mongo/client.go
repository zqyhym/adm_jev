package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Connect 建立连接并确认 Mongo 可达。库名对应的句柄先交给启动流程，暂不建集合。
func Connect(ctx context.Context, uri, database string) (*mongo.Client, *mongo.Database, error) {
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, nil, fmt.Errorf("创建 MongoDB 客户端失败: %w", err)
	}
	if err = client.Ping(connectCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, nil, fmt.Errorf("连接 MongoDB 失败: %w", err)
	}
	return client, client.Database(database), nil
}
