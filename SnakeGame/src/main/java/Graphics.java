import javax.swing.*;
import java.awt.*;
import java.awt.event.ActionEvent;
import java.awt.event.ActionListener;
import java.awt.event.KeyAdapter;
import java.awt.event.KeyEvent;

public class Graphics extends JPanel implements ActionListener {
    static final int WIDTH = 800;
    static final int HEIGHT = 800;
    static final int TICK_SIZE = 50;
    static final int BOARD_SIZE = (WIDTH * HEIGHT) / (TICK_SIZE * TICK_SIZE);
    
    final Font font = new Font("Times New Roman", Font.BOLD, 30);
    
    int[] snake_pos_X = new int[BOARD_SIZE];
    int[] snake_pos_Y = new int[BOARD_SIZE];
    int snake_length;
    
    Food food;
    int food_eaten;
    
    char direction = 'R';
    boolean is_moving = false;
    final Timer timer = new Timer(150, this);
    
    public Graphics() {
        this.setPreferredSize(new Dimension(WIDTH, HEIGHT));
        this.setBackground(Color.WHITE);
        this.setFocusable(true);
        this.addKeyListener(new KeyAdapter() {
            @Override
            public void keyPressed(KeyEvent e) {
                if (is_moving) {
                    switch (e.getKeyCode()) {
                        case KeyEvent.VK_UP:
                            if (direction != 'D') {
                                direction = 'U';
                            }
                            break;
                        case KeyEvent.VK_DOWN:
                            if (direction != 'U') {
                                direction = 'D';
                            }
                            break;
                        case KeyEvent.VK_LEFT:
                            if (direction != 'R') {
                                direction = 'L';
                            }
                            break;
                        case KeyEvent.VK_RIGHT:
                            if (direction != 'L') {
                                direction = 'R';
                            }
                            break;
                    }
                } else {
                    start();
                }
            }
        });
        
        start();
    }
    
    protected void start() {
        snake_pos_X = new int[BOARD_SIZE];
        snake_pos_Y = new int[BOARD_SIZE];
        snake_length = 5;
        food_eaten = 0;
        direction = 'R';
        is_moving = true;
        spawnFood();
        timer.start();
    }
    
    protected void move() {
        for (int i = snake_length; i > 0; i--) {
            snake_pos_X[i] = snake_pos_X[i - 1];
            snake_pos_Y[i] = snake_pos_Y[i - 1];
        }
        
        switch (direction) {
            case 'U' -> snake_pos_Y[0] -= TICK_SIZE;
            case 'D' -> snake_pos_Y[0] += TICK_SIZE;
            case 'L' -> snake_pos_X[0] -= TICK_SIZE;
            case 'R' -> snake_pos_X[0] += TICK_SIZE;
        }
    }
    
    protected void spawnFood() {
        food = new Food();
    }
    
    protected void eatFood() {
        if (snake_pos_X[0] == food.getPos_X() && snake_pos_Y[0] == food.getPos_Y()) {
            snake_length++;
            food_eaten++;
            spawnFood();
        }
    }
    
    protected void collisionTest() {
        for (int i = snake_length; i > 0; i--) {
            if (snake_pos_X[0] == snake_pos_X[i] && snake_pos_Y[0] == snake_pos_Y[i]) {
                is_moving = false;
                break;
            }
        }
        
        if (snake_pos_X[0] < 0) {
            snake_pos_X[0] = WIDTH - TICK_SIZE;
        }
        if (snake_pos_Y[0] < 0) {
            snake_pos_Y[0] = HEIGHT - TICK_SIZE;
        }
        if (snake_pos_X[0] > WIDTH - TICK_SIZE) {
            snake_pos_X[0] = 0;
        }
        if (snake_pos_Y[0] > HEIGHT - TICK_SIZE) {
            snake_pos_Y[0] = 0;
        }
        
        if (!is_moving) {
            timer.stop();
        }
    }
    
    @Override
    protected void paintComponent(java.awt.Graphics g) {
        super.paintComponent(g);
        
        if (is_moving) {
            g.setColor(Color.RED);
            g.fillOval(food.getPos_X(), food.getPos_Y(), TICK_SIZE, TICK_SIZE);
            
            g.setColor(Color.GREEN);
            for (int i = 0; i < snake_length; i++) {
                g.fillRect(snake_pos_X[i], snake_pos_Y[i], TICK_SIZE, TICK_SIZE);
            }
        } else {
            String score_text = String.format("The end... Score: %d. Press any key to play again!", food_eaten);
            g.setColor(Color.BLACK);
            g.setFont(font);
            g.drawString(score_text, (WIDTH - getFontMetrics(g.getFont()).stringWidth(score_text)) / 2, HEIGHT / 2);
        }
    }
    
    @Override
    public void actionPerformed(ActionEvent e) {
        if (is_moving) {
            move();
            collisionTest();
            eatFood();
        }
        repaint();
    }
}
