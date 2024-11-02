public class Weather {
    private final int temp;
    private final int feels_like;
    private final int humidity;
    private final int pressure;
    private final double wind_speed;
    private final String description;
    
    Weather(double temp, double feels_like, int humidity, int pressure, double wind_speed, String description) {
        this.temp = (int) temp;
        this.feels_like = (int) feels_like;
        this.humidity = humidity;
        this.pressure = pressure;
        this.wind_speed = wind_speed;
        this.description = description;
    }
    
    public String getWeatherInfo() {
        return    "\tdescription: " + description +
                "\n\ttemperature: " + temp + " C°" +
                "\n\tfeels like: " + feels_like + " C°" +
                "\n\thumidity: " + humidity + " %" +
                "\n\tpressure: " + pressure + " hPa" +
                "\n\twind speed: " + wind_speed + " m/s";
    }
}