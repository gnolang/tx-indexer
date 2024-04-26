package main

type Realm struct {
	Address     string `gorm:"index"`
	PackagePath string `gorm:"primaryKey"`
	CodeHash    []byte `gorm:"index"`
}

type User struct {
	Address string `gorm:"primaryKey"`
	Name    string `gorm:"index"`
}

var allModels = []interface{}{
	&Realm{},
	&User{},
}
