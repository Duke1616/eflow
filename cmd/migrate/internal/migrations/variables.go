package migrations

// mongoVariables MongoDB 中的环境变量源数据实体。
type mongoVariables struct {
	Key    string `bson:"key"`
	Value  string `bson:"value"`
	Secret bool   `bson:"secret"`
}
