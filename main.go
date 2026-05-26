package main

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite" // <--- Pure Go driver (No CGO)
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	DB        *gorm.DB
	DB_URL    = getEnv("DATABASE_URL", "todo.db")
	SECRET    = []byte(getEnv("SECRET_KEY", "my_secret_key"))
	ALGORITHM = jwt.SigningMethodHS256
)

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

type UserModel struct {
	ID       uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Email    string `gorm:"unique;not null" json:"email"`
	Password string `gorm:"not null" json:"-"`
}

type TodoModel struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Title     string    `gorm:"not null" json:"title"`
	Completed bool      `gorm:"default:false" json:"completed"`
	OwnerID   uint      `gorm:"not null" json:"owner_id"`
	Owner     UserModel `gorm:"foreignKey:OwnerID" json:"-"`
}

type UserCreate struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type UserResponse struct {
	ID    uint   `json:"id"`
	Email string `json:"email"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

type TodoCreate struct {
	Title string `json:"title" binding:"required"`
}

type TodoUpdate struct {
	Title     string `json:"title"`
	Completed *bool  `json:"completed"`
}

func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func verifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func createAccessToken(email string) (string, error) {
	expirationTime := time.Now().Add(30 * time.Minute)
	claims := jwt.MapClaims{
		"sub": email,
		"exp": expirationTime.Unix(),
	}
	token := jwt.NewWithClaims(ALGORITHM, claims)
	return token.SignedString(SECRET)
}

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization header format"})
			c.Abort()
			return
		}
		tokenStr := parts[1]
		claims := jwt.MapClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			return SECRET, nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}
		email, _ := claims["sub"].(string)
		var user UserModel
		if err := DB.Where("email = ?", email).First(&user).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found"})
			c.Abort()
			return
		}
		c.Set("user", user)
		c.Next()
	}
}

func register(c *gin.Context) {
	var input UserCreate
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var existing UserModel
	if err := DB.Where("email = ?", input.Email).First(&existing).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email already registered"})
		return
	}
	hashedPassword, _ := hashPassword(input.Password)
	newUser := UserModel{Email: input.Email, Password: hashedPassword}

	DB.Create(&newUser)
	c.JSON(http.StatusCreated, UserResponse{ID: newUser.ID, Email: newUser.Email})
}

func login(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	var user UserModel
	if err := DB.Where("email = ?", username).First(&user).Error; err != nil || !verifyPassword(password, user.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}
	token, err := createAccessToken(user.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create token"})
		return
	}
	c.JSON(http.StatusOK, TokenResponse{AccessToken: token, TokenType: "bearer"})
}

func createTodo(c *gin.Context) {
	currentUser := c.MustGet("user").(UserModel)
	var input TodoCreate
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	newTodo := TodoModel{Title: input.Title, OwnerID: currentUser.ID}
	DB.Create(&newTodo)
	c.JSON(http.StatusCreated, newTodo)
}

func readTodos(c *gin.Context) {
	currentUser := c.MustGet("user").(UserModel)
	var todos []TodoModel
	DB.Where("owner_id = ?", currentUser.ID).Find(&todos)
	c.JSON(http.StatusOK, todos)
}

func updateTodo(c *gin.Context) {
	currentUser := c.MustGet("user").(UserModel)
	todoID := c.Param("id")
	var input TodoUpdate
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var todo TodoModel
	if err := DB.Where("id = ? AND owner_id = ?", todoID, currentUser.ID).First(&todo).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Todo not found"})
		return
	}

	if input.Title != "" {
		todo.Title = input.Title
	}
	if input.Completed != nil {
		todo.Completed = *input.Completed
	}

	DB.Save(&todo)
	c.JSON(http.StatusOK, todo)
}

func deleteTodo(c *gin.Context) {
	currentUser := c.MustGet("user").(UserModel)
	todoID := c.Param("id")
	var todo TodoModel
	if err := DB.Where("id = ? AND owner_id = ?", todoID, currentUser.ID).First(&todo).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Todo not found"})
		return
	}
	DB.Delete(&todo)
	c.Status(http.StatusNoContent)
}

func main() {
	var err error
	DB, err = gorm.Open(sqlite.Open(DB_URL), &gorm.Config{})
	if err != nil {
		panic("Failed to connect to database: " + err.Error())
	}
	DB.AutoMigrate(&UserModel{}, &TodoModel{})
	r := gin.Default()
	r.POST("/register", register)
	r.POST("/login", login)

	todoRoutes := r.Group("/todos")
	todoRoutes.Use(authMiddleware())
	{
		todoRoutes.POST("", createTodo)
		todoRoutes.GET("/read", readTodos)
		todoRoutes.PUT("/:id", updateTodo)
		todoRoutes.DELETE("/:id", deleteTodo)
	}
	r.Run(":8000")
}
