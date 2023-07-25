package seed

import (
	"log"

	"Matchup/api/models"

	"github.com/jinzhu/gorm"
)

var users = []models.User{
	{
		Username: "steven",
		Email:    "steven@example.com",
		Password: "password",
	},
	{
		Username: "martin",
		Email:    "luther@example.com",
		Password: "password",
	},
}

var matchups = []models.Matchup{
	{
		Title:   "Title 1",
		Content: "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum",
	},
	{
		Title:   "Title 2",
		Content: "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum",
	},
}

func Load(db *gorm.DB) {

	err := db.Debug().DropTableIfExists(&models.Matchup{}, &models.User{}, &models.Like{}, &models.Comment{}).Error
	if err != nil {
		log.Fatalf("cannot drop table: %v", err)
	}
	err = db.Debug().AutoMigrate(&models.User{}, &models.Matchup{}).Error
	if err != nil {
		log.Fatalf("cannot migrate table: %v", err)
	}

	err = db.Debug().Model(&models.Matchup{}).AddForeignKey("author_id", "users(id)", "cascade", "cascade").Error
	if err != nil {
		log.Fatalf("attaching foreign key error: %v", err)
	}

	for i, _ := range users {
		err = db.Debug().Model(&models.User{}).Create(&users[i]).Error
		if err != nil {
			log.Fatalf("cannot seed users table: %v", err)
		}
		matchups[i].AuthorID = users[i].ID

		err = db.Debug().Model(&models.Matchup{}).Create(&matchups[i]).Error
		if err != nil {
			log.Fatalf("cannot seed matchups table: %v", err)
		}
	}
}
