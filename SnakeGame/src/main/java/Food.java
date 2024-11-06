import java.util.Random;

public class Food {

    private final int pos_X;
    private final int pos_Y;
    
    public Food() {
        this.pos_X = generateFoodPos(Graphics.WIDTH);
        this.pos_Y = generateFoodPos(Graphics.HEIGHT);
    }
    
    private int generateFoodPos(int size) {
        return new Random().nextInt(size / Graphics.TICK_SIZE) * Graphics.TICK_SIZE;
    }
    
    public int getPos_X() {
        return pos_X;
    }
    
    public int getPos_Y() {
        return pos_Y;
    }
}
