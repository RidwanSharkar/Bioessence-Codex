package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/RidwanSharkar/Bioessence/backend/models"
	"github.com/RidwanSharkar/Bioessence/backend/services"
	"github.com/RidwanSharkar/Bioessence/backend/utils"
	"github.com/joho/godotenv"
	"github.com/rs/cors"
)

/*==================================================================================*/

// List of essential nutrients (example)
var essentialNutrients = []string{
	"Calcium",
	"Iron",
	"Magnesium",
	"Potassium",
	"Vitamin C",
	"Vitamin D",
	// Add more as needed
}

func main() {
	// Load .env file
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file:", err)
	}

	// Set up CORS
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"http://localhost:5173"}, // Allow requests from React app
		AllowedMethods: []string{"POST", "GET", "OPTIONS", "PUT", "DELETE"},
		AllowedHeaders: []string{"Accept", "Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization"},
	})

	// Define the HTTP endpoint
	http.HandleFunc("/process-food", processFoodHandler)

	// Wrap the multiplexer with CORS middleware
	handler := c.Handler(http.DefaultServeMux)

	// Start the server
	fmt.Println("Server is running on :5000")
	log.Fatal(http.ListenAndServe(":5000", handler))
}

func processFoodHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.RespondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req models.FoodRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, "Error extracting ingredients: "+err.Error())
		return
	}

	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		utils.RespondWithError(w, http.StatusInternalServerError, "API_KEY not set")
		return
	}

	promptText := fmt.Sprintf("Extract the essential ingredients from the following food description: '%s'. For complex foods like pizza, include the base components (e.g., dough, cheese, tomato sauce). Exclude spices and minor ingredients.", req.FoodDescription)

	// Extract ingredients using Gemini LLM
	ingredients, err := extractIngredientsFromGemini(apiKey, promptText)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, "Error extracting ingredients: "+err.Error())
		return
	}

	cleanedIngredients := cleanIngredientList(ingredients)

	// Fetch nutrient data for each cleaned ingredient - Nutritionix
	nutrientData, err := services.FetchNutrientDataForEachIngredient(cleanedIngredients)
	if err != nil {
		http.Error(w, "Error fetching nutrient data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Aggregate Data
	aggregatedNutrients := aggregateNutrients(nutrientData)
	missingNutrients := determineMissingNutrients(aggregatedNutrients)

	// Generate Suggestions
	suggestions := generateSuggestions(missingNutrients)

	// Prepare the response using models.ProcessFoodResponse
	response := models.ProcessFoodResponse{
		Ingredients:      cleanedIngredients,
		Nutrients:        aggregatedNutrients,
		MissingNutrients: missingNutrients,
		Suggestions:      suggestions,
	}

	// Send the response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Function to extract Gemini output
func extractIngredientsFromGemini(apiKey, prompt string) (string, error) {
	// Create the request payload
	reqBody := models.GeminiRequest{
		Contents: []models.Content{
			{
				Parts: []models.Part{
					{Text: prompt},
				},
			},
		},
	}

	// Convert the request payload to JSON
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	// Send the request to the Gemini API
	endpoint := "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash-latest:generateContent?key=" + apiKey
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Read the response body
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Decode the JSON response
	var geminiResp models.GeminiResponse
	err = json.Unmarshal(bodyBytes, &geminiResp)
	if err != nil {
		return "", err
	}

	// Extract the ingredients from the response
	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		return geminiResp.Candidates[0].Content.Parts[0].Text, nil
	}

	return "", fmt.Errorf("no ingredients returned from Gemini")
}

// Extract True Ingredients
func cleanIngredientList(ingredients string) []string {
	lines := strings.Split(ingredients, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.ReplaceAll(line, "*", "")
		line = strings.TrimPrefix(line, "• ")
		if len(line) > 0 && !strings.Contains(line, ":") && len(line) < 50 {
			cleaned = append(cleaned, line)
		}
	}
	return cleaned
}

// Aggregate Nutrition Data
func aggregateNutrients(nutrientData map[string]map[string]float64) map[string]map[string]float64 {
	// Since nutrientData already maps each ingredient to its nutrients,
	// you can directly return it. If additional aggregation is needed, implement here.
	return nutrientData
}

// Determine Missing Nutrients
func determineMissingNutrients(nutrientData map[string]map[string]float64) []string {
	var missing []string
	nutrientSet := make(map[string]bool)
	for _, nutrients := range nutrientData {
		for nutrient := range nutrients {
			nutrientSet[nutrient] = true
		}
	}
	for _, nutrient := range essentialNutrients {
		if !nutrientSet[nutrient] {
			missing = append(missing, nutrient)
		}
	}
	return missing
}

// Generate Suggestions Based on Missing Nutrients
func generateSuggestions(missing []string) []string {
	suggestionsMap := map[string]string{
		"Vitamin D": "Include more fatty fish or fortified dairy products.",
		"Calcium":   "Consider adding more leafy greens or dairy products.",
		// Add more mappings as needed
	}

	var suggestions []string
	for _, nutrient := range missing {
		if suggestion, exists := suggestionsMap[nutrient]; exists {
			suggestions = append(suggestions, suggestion)
		} else {
			suggestions = append(suggestions, fmt.Sprintf("Consider adding sources rich in %s.", nutrient))
		}
	}
	return suggestions
}
