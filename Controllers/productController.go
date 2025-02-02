package Controllers

import (
	"context"
	"mime/multipart"
	"net/http"
	"strconv"

	"Server/Middleware"
	"Server/Models"

	"github.com/cloudinary/cloudinary-go"
	"github.com/cloudinary/cloudinary-go/api/uploader"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func uploadToCloudinary(file multipart.File, fileName string) (string, error) {
	cld, err := cloudinary.NewFromParams("dflhancsp", "437865386617669", "uLJSc-9ItdeXSbWyEndQ3x-F1FY")
	if err != nil {
		return "", err
	}
	ctx := context.Background()
	uploadResult, err := cld.Upload.Upload(ctx, file, uploader.UploadParams{PublicID: fileName})
	if err != nil {
		return "", err
	}
	return uploadResult.SecureURL, nil
}

func CreateProduct(c *gin.Context) {
	claims := c.MustGet("user").(*Middleware.UserClaims)

	if claims.Role != Middleware.Admin && claims.Role != Middleware.Staff {
		c.JSON(403, gin.H{"error": "You are not authorized to create products"})
		return
	}

	var product Models.Product

	err := c.Request.ParseMultipartForm(10 << 20)
	if err != nil {
		c.JSON(400, gin.H{"error": "Could not parse multipart form"})
		return
	}

	file, err := c.FormFile("image")
	if err != nil {
		c.JSON(400, gin.H{"error": "Could not get file from form"})
		return
	}

	fileContent, err := file.Open()
	if err != nil {
		c.JSON(500, gin.H{"error": "Could not open file"})
		return
	}
	defer fileContent.Close()

	url, err := uploadToCloudinary(fileContent, file.Filename)
	if err != nil {
		c.JSON(500, gin.H{"error": "Could not upload image to Cloudinary"})
		return
	}

	product.ImageURL = url
	product.Name = c.PostForm("name")
	product.Price, _ = strconv.ParseFloat(c.PostForm("price"), 64)
	product.Stock, _ = strconv.Atoi(c.PostForm("stock"))
	product.ProductCategory, _ = primitive.ObjectIDFromHex(c.PostForm("productcategory"))

	if product.Name == "" || product.Price <= 0 || product.Stock <= 0 {
		c.JSON(400, gin.H{"error": "Invalid input"})
		return
	}

	product.ID = primitive.NewObjectID()

	collection := getCollection("products")
	if _, err := collection.InsertOne(context.Background(), product); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, product)
}

func GetAllProducts(c *gin.Context) {
	var products []Models.Product
	collection := getCollection("products")
	cursor, err := collection.Find(context.Background(), bson.M{})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer cursor.Close(context.Background())

	for cursor.Next(context.Background()) {
		var product Models.Product
		cursor.Decode(&product)
		products = append(products, product)
	}

	c.JSON(200, products)
}

func GetProductByID(c *gin.Context) {
	id := c.Param("id")
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid ID format"})
		return
	}

	var product Models.Product
	collection := getCollection("products")
	if err = collection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&product); err != nil {
		c.JSON(404, gin.H{"error": "Product not found"})
		return
	}

	c.JSON(200, product)
}

func UpdateProduct(c *gin.Context) {
	claims := c.MustGet("user").(*Middleware.UserClaims)

	if claims.Role != Middleware.Admin && claims.Role != Middleware.Staff {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are not authorized to update products"})
		return
	}

	id := c.Param("id")
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID format"})
		return
	}

	var existingProduct Models.Product
	collection := getCollection("products")
	err = collection.FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&existingProduct)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		return
	}

	err = c.Request.ParseMultipartForm(10 << 20)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not parse multipart form"})
		return
	}

	file, err := c.FormFile("image")
	if err == nil {
		fileContent, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not open file"})
			return
		}
		defer fileContent.Close()

		url, err := uploadToCloudinary(fileContent, file.Filename)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not upload image to Cloudinary"})
			return
		}

		existingProduct.ImageURL = url
	}

	if name := c.PostForm("name"); name != "" {
		existingProduct.Name = name
	}
	if price, err := strconv.ParseFloat(c.PostForm("price"), 64); err == nil && price > 0 {
		existingProduct.Price = price
	}
	if stock, err := strconv.Atoi(c.PostForm("stock")); err == nil && stock >= 0 {
		existingProduct.Stock = stock
	}
	if category := c.PostForm("productcategory"); category != "" {
		if productCategory, err := primitive.ObjectIDFromHex(category); err == nil {
			existingProduct.ProductCategory = productCategory
		}
	}

	if existingProduct.Name == "" || existingProduct.Price <= 0 || existingProduct.Stock < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	update := bson.M{
		"$set": bson.M{
			"name":            existingProduct.Name,
			"price":           existingProduct.Price,
			"stock":           existingProduct.Stock,
			"productcategory": existingProduct.ProductCategory,
			"imageurl":        existingProduct.ImageURL,
		},
	}

	result, err := collection.UpdateOne(context.Background(), bson.M{"_id": objectID}, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		return
	}

	c.JSON(http.StatusOK, existingProduct)
}

func DeleteProduct(c *gin.Context) {
	claims := c.MustGet("user").(*Middleware.UserClaims)

	if claims.Role != Middleware.Admin {
		c.JSON(403, gin.H{"error": "You are not authorized to delete products"})
		return
	}

	id := c.Param("id")
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid ID format"})
		return
	}

	collection := getCollection("products")
	if result, err := collection.DeleteOne(context.Background(), bson.M{"_id": objectID}); err != nil || result.DeletedCount == 0 {
		c.JSON(404, gin.H{"error": "Product not found"})
		return
	}

	c.Status(204)
}
