import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.util.ArrayList;
import java.util.List;
import java.util.Scanner;
import java.util.concurrent.CompletableFuture;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

public class PlacesApp {
    private static final String GEOCODE_API = "https://graphhopper.com/api/1/geocode?q=";
    private static final String geo_apiKey = "ff02c803-dede-4557-8d3e-851d1a1132ef";
    
    private static final String WEATHER_API = "https://api.openweathermap.org/data/2.5/weather?";
    private static final String weather_apiKey = "31e818e0682ca6efac18ce313700e7d9";
    
    private static final String PLACES_API = "https://api.geoapify.com/v2/places?categories=entertainment,activity";
    private static final String places_apiKey = "d45805d2e9f94b4b8869d2479115d561";
    private static final int circleRadiusMeters = 5000;
    
    private static final String DETAILS_API = "https://api.geoapify.com/v2/place-details?";
    private static final String details_apiKey = "d45805d2e9f94b4b8869d2479115d561";
    
    private static final int limit = 10;
    private static final HttpClient httpClient = HttpClient.newHttpClient();
    private static final ObjectMapper objectMapper = new ObjectMapper();
    
    public static void main(String[] args) {
        System.out.print("Enter location: ");
        Scanner scanner = new Scanner(System.in);
        String userInputLocation = null;
        
        while (userInputLocation == null) {
            if (scanner.hasNextLine())
                userInputLocation = scanner.nextLine();
            else
                System.out.println("Couldn't parse input, please try again.");
        }
        
        userInputLocation = userInputLocation.replaceAll(" ", "+");
        
        CompletableFuture<List<Location>> locationFuture = searchLocations(userInputLocation);
        
        locationFuture.thenCompose(locations -> {
            System.out.println("Please, choose one of the following locations:");
            
            int i = 1;
            for (Location location : locations) {
                System.out.println("Location " + i++ + ":");
                location.getLocationInfo();
            }
            
            i = 0;
            while (i < 1 || i >= locations.size()) {
                System.out.print("You choose: ");
                if (scanner.hasNextInt())
                    i = scanner.nextInt();
                if (i < 1 || i >= locations.size())
                    System.out.println("Wrong input, please try again.");
            }
            
            Location chosenLocation = locations.get(i - 1);
            
            CompletableFuture<Weather> weatherFuture = getWeather(chosenLocation);
            CompletableFuture<List<Place>> placesFuture = getPlaces(chosenLocation);
            
            return weatherFuture.thenCombine(placesFuture, (weather, places) -> {
                System.out.println("The weather in " + chosenLocation.getName() + ": \n" + weather.getWeatherInfo());
                System.out.println("//--------------------------------------------------//\n" +
                                    "Interesting places nearby:");
                return CompletableFuture.allOf(places.stream()
                                .map(place -> getPlaceDetails(place).thenAccept(details -> {
                                    System.out.println("\tPlace: " + place.name());
                                    System.out.println("\t  Description:");
                                    details.getDescription();
                                }))
                                .toArray(CompletableFuture[]::new)
                );
            }).thenCompose(future -> future);
        }).join();
    }
    
    public static CompletableFuture<List<Location>> searchLocations(String query) {
        String uri = GEOCODE_API + query + "&locale=default&limit=" + limit + "&key=" + geo_apiKey;
        HttpRequest request = HttpRequest.newBuilder().uri(URI.create(uri)).build();
        
        return httpClient.sendAsync(request, HttpResponse.BodyHandlers.ofString()).thenApply(response -> {
                    try {
                        JsonNode jsonNode = objectMapper.readTree(response.body());
                        List<Location> locations = parseLocations(jsonNode);
                        if (locations == null)
                            throw new NullPointerException();
                        return parseLocations(jsonNode);
                    } catch (NullPointerException e) {
                        throw new RuntimeException("Couldn't find any locations by the given location name.", e);
                    } catch (Exception e) {
                        throw new RuntimeException("Error occurred while searching the location.", e);
                    }
                });
    }
    
    public static CompletableFuture<Weather> getWeather(Location location) {
        String uri = WEATHER_API + location.getCoordinates() + "&appid=" + weather_apiKey + "&units=metric";
        HttpRequest request = HttpRequest.newBuilder().uri(URI.create(uri)).build();
        
        return httpClient.sendAsync(request, HttpResponse.BodyHandlers.ofString()).thenApply(response -> {
                    try {
                        JsonNode jsonNode = objectMapper.readTree(response.body());
                        return parseWeather(jsonNode);
                    } catch (Exception e) {
                        throw new RuntimeException("Error occurred while getting the weather information.", e);
                    }
                });
    }
    
    public static CompletableFuture<List<Place>> getPlaces(Location location) {
        String uri = PLACES_API + "&filter=circle:" + location.getLng() + "," + location.getLat() + "," + circleRadiusMeters +
                        "&limit=" + limit + "&apiKey=" + places_apiKey;
        HttpRequest request = HttpRequest.newBuilder().uri(URI.create(uri)).build();
        
        return httpClient.sendAsync(request, HttpResponse.BodyHandlers.ofString()).thenApply(response -> {
                    try {
                        JsonNode jsonNode = objectMapper.readTree(response.body());
                        return parsePlaces(jsonNode);
                    } catch (Exception e) {
                        throw new RuntimeException("Error occurred while getting the interesting places.", e);
                    }
                });
    }
    
    public static CompletableFuture<PlaceDetails> getPlaceDetails(Place place) {
        String uri = DETAILS_API + "id=" + place.id() + "&apiKey=" + details_apiKey;
        HttpRequest request = HttpRequest.newBuilder().uri(URI.create(uri)).build();
        
        return httpClient.sendAsync(request, HttpResponse.BodyHandlers.ofString()).thenApply(response -> {
                    try {
                        JsonNode jsonNode = objectMapper.readTree(response.body());
                        return parsePlaceDetails(jsonNode);
                    } catch (Exception e) {
                        throw new RuntimeException("Error occurred while getting the place details.", e);
                    }
                });
    }
    
    public static Location getLocationFromHit(JsonNode hit) {
        JsonNode point = hit.get("point");
        
        double lat = (point.has("lat")) ? point.get("lat").asDouble() : 0.0;
        double lng = (point.has("lng")) ? point.get("lng").asDouble() : 0.0;
        
        String name =     (hit.has("name"))      ? hit.get("name").asText() : null;
        String country =  (hit.has("country"))   ? hit.get("country").asText() : null;
        String city =     (hit.has("city"))      ? hit.get("city").asText() : null;
        String osmValue = (hit.has("osm_value")) ? hit.get("osm_value").textValue() : null;
        
        return new Location(name, lat, lng, country, city, osmValue);
    }
    
    public static Place getPlaceFromFeature(JsonNode feature) {
        JsonNode properties = feature.get("properties");
        
        String name = (properties.has("name"))     ? properties.get("name").asText() : null;
        String id =   (properties.has("place_id")) ? properties.get("place_id").asText() : null;
        
        return new Place(name, id);
    }
    
    public static List<Location> parseLocations(JsonNode jsonNode) {
        if (jsonNode.has("hits")) {
            List<Location> locations = new ArrayList<>();
            JsonNode arrayNode = jsonNode.get("hits");
            for (int i = 0; i < arrayNode.size(); i++)
                locations.add(getLocationFromHit(arrayNode.get(i)));
            return (locations.isEmpty()) ? null : locations;
        }
        return null;
    }
    
    public static Weather parseWeather(JsonNode jsonNode) {
        if (jsonNode.has("weather") && jsonNode.has("main")) {
            String description = jsonNode.has("weather") ? jsonNode.get("weather").get(0).get("description").asText() : null;
            
            double temp =        jsonNode.has("main")    ? jsonNode.get("main").get("temp").asDouble() : 0.0;
            double feels_like =  jsonNode.has("main")    ? jsonNode.get("main").get("feels_like").asDouble() : 0.0;
            double wind_speed =  jsonNode.has("wind")    ? jsonNode.get("wind").get("speed").asDouble() : 0.0;
            
            int pressure =       jsonNode.has("main")    ? jsonNode.get("main").get("pressure").asInt() : 0;
            int humidity =       jsonNode.has("main")    ? jsonNode.get("main").get("humidity").asInt() : 0;
            
            return new Weather(temp, feels_like, humidity, pressure, wind_speed, description);
        }
        return null;
    }
    
    public static List<Place> parsePlaces(JsonNode jsonNode) {
        if (jsonNode.has("features")) {
            List<Place> places = new ArrayList<>();
            JsonNode arrayNode = jsonNode.get("features");
            for (int i = 0; i < arrayNode.size(); i++)
                places.add(getPlaceFromFeature(arrayNode.get(i)));
            return (places.isEmpty()) ? null : places;
        }
        return null;
    }
    
    public static PlaceDetails parsePlaceDetails(JsonNode jsonNode) {
        if (jsonNode.has("features")) {
            JsonNode properties = jsonNode.get("features").get(0).get("properties");
            String address = properties.has("address_line2") ? properties.get("address_line2").asText() : null;
            
            JsonNode datasource = properties.get("datasource").get("raw");
            
            String email =        datasource.has("email")         ? datasource.get("email").asText() : null;
            String phone =        datasource.has("phone")         ? datasource.get("phone").asText() : null;
            String website =      datasource.has("website")       ? datasource.get("website").asText() : null;
            String openingHours = datasource.has("opening_hours") ? datasource.get("opening_hours").asText() : null;
            
            return new PlaceDetails(address, email, phone, website, openingHours);
        }
        return null;
    }
}
