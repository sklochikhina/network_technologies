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
    
    public void getDescription() {
        int i = 1;
        if (address != null)      System.out.println("\t\t" + i++ + ". address: " + address);
        if (email != null)        System.out.println("\t\t" + i++ + ". email: " + email);
        if (phone != null)        System.out.println("\t\t" + i++ + ". phone: " + phone);
        if (website != null)      System.out.println("\t\t" + i++ + ". website: " + website);
        if (openingHours != null) System.out.println("\t\t" + i + ". openingHours: " + openingHours);
    }
}