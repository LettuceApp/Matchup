package models

import (
	"context"
	"errors"
	"html"
	"strings"
	"time"

	appdb "Matchup/db"

	"github.com/jmoiron/sqlx"
)

type ItemPool struct {
	ID          uint      `db:"id" json:"id"`
	PublicID    string    `db:"public_id" json:"public_id"`
	ShortID     *string   `db:"short_id" json:"short_id,omitempty"`
	Title       string    `db:"title" json:"title"`
	Description string    `db:"description" json:"description"`
	Author      User      `db:"-" json:"-"`
	AuthorID    uint      `db:"author_id" json:"author_id"`
	Size        int       `db:"size" json:"size"`
	Visibility  string    `db:"visibility" json:"visibility"`
	Items       []ItemPoolItem `db:"-" json:"items"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

type ItemPoolItem struct {
	ID         uint      `db:"id" json:"id"`
	ItemPoolID uint      `db:"item_pool_id" json:"item_pool_id"`
	Item       string    `db:"item" json:"item"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
}

func (ip *ItemPool) Prepare() {
	ip.Title = html.EscapeString(strings.TrimSpace(ip.Title))
	ip.Description = html.EscapeString(strings.TrimSpace(ip.Description))
	ip.Author = User{}
	ip.CreatedAt = time.Now()
	ip.UpdatedAt = time.Now()

	if strings.TrimSpace(ip.Visibility) == "" {
		ip.Visibility = "public"
	}
}

func (ip *ItemPool) Validate() map[string]string {
	var err error
	var errorMessages = make(map[string]string)

	if ip.Title == "" {
		err = errors.New("required title")
		errorMessages["Required_title"] = err.Error()
	}
	if ip.AuthorID == 0 {
		err = errors.New("required author")
		errorMessages["Required_author"] = err.Error()
	}
	if len(ip.Items) > 100 {
		err = errors.New("item pool cannot exceed 100 items")
		errorMessages["size_limit"] = err.Error()
	}
	return errorMessages
}

func (ip *ItemPool) SaveItemPool(db sqlx.ExtContext) (*ItemPool, error) {
	if strings.TrimSpace(ip.PublicID) == "" {
		ip.PublicID = appdb.GeneratePublicID()
	}

	ip.Size = len(ip.Items)

	query, args, err := appdb.Psql.Insert("item_pools").
		Columns("public_id", "short_id", "title", "description", "author_id", "size", "visibility", "created_at", "updated_at").
		Values(ip.PublicID, ip.ShortID, ip.Title, ip.Description, ip.AuthorID, ip.Size, ip.Visibility, ip.CreatedAt, ip.UpdatedAt).
		Suffix("RETURNING *").
		ToSql()
	if err != nil {
		return nil, err
	}
	
	if err := sqlx.GetContext(context.Background(), db, ip, query, args...); err != nil {
		return nil, err
	}

	// Save items
	for i, item := range ip.Items {
		if item.ID == 0 {
			ip.Items[i].ItemPoolID = ip.ID
			q, a, err := appdb.Psql.Insert("item_pool_items").
				Columns("item_pool_id", "item", "created_at").
				Values(ip.Items[i].ItemPoolID, ip.Items[i].Item, time.Now()).
				Suffix("RETURNING *").
				ToSql()
			if err != nil {
				return nil, err
			}
			if err := sqlx.GetContext(context.Background(), db, &ip.Items[i], q, a...); err != nil {
				return nil, err
			}
		}
	}

	// Load author
	if err := sqlx.GetContext(context.Background(), db, &ip.Author, "SELECT * FROM users WHERE id = $1", ip.AuthorID); err != nil {
		return nil, err
	}
	ip.Author.ProcessAvatarPath()

	return ip, nil
}

func (ip *ItemPool) FindItemPoolByID(db sqlx.ExtContext, id uint) (*ItemPool, error) {
	err := sqlx.GetContext(context.Background(), db, ip, "SELECT * FROM item_pools WHERE id = $1", id)
	if err != nil {
		return nil, err
	}
	
	err = sqlx.SelectContext(context.Background(), db, &ip.Items, "SELECT * FROM item_pool_items WHERE item_pool_id = $1 ORDER BY id ASC", id)
	if err != nil {
		return nil, err
	}
	
	if err := sqlx.GetContext(context.Background(), db, &ip.Author, "SELECT * FROM users WHERE id = $1", ip.AuthorID); err != nil {
		return nil, err
	}
	ip.Author.ProcessAvatarPath()
	return ip, nil
}

func (ip *ItemPool) UpdateItemPool(db sqlx.ExtContext) (*ItemPool, error) {
	ip.UpdatedAt = time.Now()
	ip.Size = len(ip.Items)

	_, err := db.ExecContext(context.Background(),
		"UPDATE item_pools SET title = $1, description = $2, visibility = $3, size = $4, updated_at = $5 WHERE id = $6",
		ip.Title, ip.Description, ip.Visibility, ip.Size, ip.UpdatedAt, ip.ID)
	if err != nil {
		return &ItemPool{}, err
	}

	// For simplicity with updating items: delete old ones and insert new ones
	_, err = db.ExecContext(context.Background(), "DELETE FROM item_pool_items WHERE item_pool_id = $1", ip.ID)
	if err != nil {
		return &ItemPool{}, err
	}

	for i, item := range ip.Items {
		item.ItemPoolID = ip.ID
		q, a, err := appdb.Psql.Insert("item_pool_items").
			Columns("item_pool_id", "item", "created_at").
			Values(item.ItemPoolID, item.Item, time.Now()).
			Suffix("RETURNING *").
			ToSql()
		if err != nil {
			return &ItemPool{}, err
		}
		if err := sqlx.GetContext(context.Background(), db, &item, q, a...); err != nil {
			return &ItemPool{}, err
		}
		ip.Items[i] = item
	}

	if ip.ID != 0 {
		err = sqlx.GetContext(context.Background(), db, &ip.Author, "SELECT * FROM users WHERE id = $1", ip.AuthorID)
		if err != nil {
			return &ItemPool{}, err
		}
		ip.Author.ProcessAvatarPath()
	}
	return ip, nil
}

func (ip *ItemPool) DeleteItemPool(db sqlx.ExtContext, id uint) (int64, error) {
	result, err := db.ExecContext(context.Background(), "DELETE FROM item_pools WHERE id = $1", id)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (ip *ItemPool) FindUserItemPools(db sqlx.ExtContext, uid uint) (*[]ItemPool, error) {
	var pools []ItemPool
	err := sqlx.SelectContext(context.Background(), db, &pools,
		"SELECT * FROM item_pools WHERE author_id = $1 ORDER BY created_at DESC", uid)
	if err != nil {
		return nil, err
	}
	
	// Optional: Could load authors/items in batch if needed. But author is known (uid).
	if len(pools) > 0 {
		var author User
		if err := sqlx.GetContext(context.Background(), db, &author, "SELECT * FROM users WHERE id = $1", uid); err != nil {
			return nil, err
		}
		author.ProcessAvatarPath()
		
		poolIDs := make([]uint, len(pools))
		for i := range pools {
			pools[i].Author = author
			poolIDs[i] = pools[i].ID
		}
		
		// Load items in batch
		query, args, err := sqlx.In("SELECT * FROM item_pool_items WHERE item_pool_id IN (?) ORDER BY id ASC", poolIDs)
		if err != nil {
			return nil, err
		}
		var allItems []ItemPoolItem
		if err := sqlx.SelectContext(context.Background(), db, &allItems, db.Rebind(query), args...); err != nil {
			return nil, err
		}
		
		itemsMap := make(map[uint][]ItemPoolItem)
		for _, item := range allItems {
			itemsMap[item.ItemPoolID] = append(itemsMap[item.ItemPoolID], item)
		}
		
		for i := range pools {
			pools[i].Items = itemsMap[pools[i].ID]
		}
	}
	
	return &pools, nil
}
