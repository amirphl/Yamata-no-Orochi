package models

type SrcLayerAllStats struct {
	Layer1Category *string  `gorm:"column:layer1_category"`
	Layer2Category *string  `gorm:"column:layer2_category"`
	Layer3Category *string  `gorm:"column:layer3_category"`
	P33            *float64 `gorm:"column:p33"`
	P66            *float64 `gorm:"column:p66"`
}

func (SrcLayerAllStats) TableName() string {
	return "src_layer_all_stats"
}
