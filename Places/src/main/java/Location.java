public class Location {
    private final double lat;
    private final double lng;
    
    private final String name;
    private final String country;
    private final String city;
    private final String osm_value;
    
    public Location(String name, double lat, double lng, String country, String city, String osm_value) {
        this.name = name;
        this.lat = lat;
        this.lng = lng;
        this.country = country;
        this.city = city;
        this.osm_value = osm_value;
    }
    
    public void getLocationInfo() {
        if (name != null)      System.out.println("\tname: " + name);
        if (osm_value != null) System.out.println("\tosm_value: " + osm_value);
        if (country != null)   System.out.println("\tcountry: " + country);
        if (city != null)      System.out.println("\tcity: " + city);
    }
    
    public String getCoordinates() {
        return "lat=" + lat + "&lon=" + lng;
    }
    
    public double getLat() {
        return lat;
    }
    
    public double getLng() {
        return lng;
    }
    
    public String getName() {
        return name;
    }
}