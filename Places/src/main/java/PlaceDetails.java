public class PlaceDetails {
    private final String address;
    private final String email;
    private final String phone;
    private final String website;
    private final String openingHours;
    
    PlaceDetails(String address, String email, String phone, String website, String openingHours) {
        this.address = address;
        this.email = email;
        this.phone = phone;
        this.website = website;
        this.openingHours = openingHours;
    }
    
    public String getDescription() {
        int i = 1;
        String description = "";
        if (address != null)      description += "\t\t" + i++ + ". address: " + address + "\n";
        if (email != null)        description += "\t\t" + i++ + ". email: " + email + "\n";
        if (phone != null)        description += "\t\t" + i++ + ". phone: " + phone + "\n";
        if (website != null)      description += "\t\t" + i++ + ". website: " + website + "\n";
        if (openingHours != null) description += "\t\t" + i + ". openingHours: " + openingHours + "\n";
        return description;
    }
}