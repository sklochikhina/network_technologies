import java.io.IOException;
import java.net.*;
import java.time.Instant;
import java.util.Arrays;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;

public class MulticastDiscovery {
    private static final int PORT = 1234;           // Порт для обмена сообщениями
    private static final long TIMEOUT = 5000;       // Тайм-аут для удаления неактивных пиров
    private static final long SEND_INTERVAL = 2000; // Интервал отправки сообщений
    private static int activePeersCount = 0;        // Количество активных пиров
    
    private static final ConcurrentHashMap<String, PeerInfo> activePeers = new ConcurrentHashMap<>();
    
    private static final UUID uniqueID = UUID.randomUUID();
    
    public static void main(String[] args) throws IOException {
        if (args.length != 1) {
            System.err.println("Usage: java self_copies\\src\\MulticastDiscovery.java <multicast-address>");
            return;
        }
        
        String multicastAddr = args[0];
        InetAddress group = InetAddress.getByName(multicastAddr);
        
        if (!(group instanceof Inet4Address || group instanceof Inet6Address)) {
            System.err.println("Wrong format of multicast address.");
            return;
        }
        
        System.out.println("My uniqueID: " + uniqueID);
        
        try (MulticastSocket socket = new MulticastSocket(PORT)) {
            socket.setReuseAddress(true);
            socket.setTimeToLive(1);
            
            socket.joinGroup(group); // set interface
            
            Thread receiveThread = new Thread(() -> receiveMessages(socket));
            receiveThread.start();
            
            Thread sendThread = new Thread(() -> sendMessages(socket, group));
            sendThread.start();
            
            while (true) {
                removeInactivePeers();
                printActivePeers();
            }
        }
    }
    
    private static void sendMessages(MulticastSocket socket, InetAddress group) {
        String message = "Hello, I'm alive! ID: " + uniqueID;
        
        while (true) {
            try {
                byte[] buf = message.getBytes();
                DatagramPacket packet = new DatagramPacket(buf, buf.length, group, PORT);
                socket.send(packet);
                Thread.sleep(SEND_INTERVAL);
            } catch (IOException | InterruptedException e) {
                e.printStackTrace();
            }
        }
    }
    
    private static void receiveMessages(MulticastSocket socket) {
        byte[] buffer = new byte[256];
        
        while (true) {
            try {
                DatagramPacket packet = new DatagramPacket(buffer, buffer.length);
                socket.receive(packet);
                
                String senderAddress = packet.getAddress().getHostAddress();
                String message = new String(Arrays.copyOfRange(packet.getData(), 0, packet.getLength()));
                System.out.println("Received message \"" + message + "\" from " + senderAddress);
                
                String peerUUID = message.substring(21);
                activePeers.put(peerUUID, new PeerInfo(senderAddress, Instant.now()));
            } catch (IOException e) {
                e.printStackTrace();
            }
        }
    }
    
    private static void removeInactivePeers() {
        Instant now = Instant.now();
        activePeers.entrySet().removeIf(entry -> {
            PeerInfo info = entry.getValue();
            long elapsed = now.toEpochMilli() - info.lastSeen.toEpochMilli();
            return elapsed > TIMEOUT;
        });
    }
    
    private static void printActivePeers() {
        // System.out.println("activePeers.size: " + activePeers.size() + " ; activePeersCount: " + activePeersCount);
        synchronized (activePeers) {
            if (activePeers.size() != activePeersCount) {
                activePeersCount = activePeers.size();
                System.out.println("Active peers:");
                activePeers.forEach((uuid, info) -> System.out.println("UUID: " + uuid + " IP: " + info.ipAddress));
            }
        }
    }
    private static class PeerInfo {
        String ipAddress;
        Instant lastSeen;
        
        PeerInfo(String ipAddress, Instant lastSeen) {
            this.ipAddress = ipAddress;
            this.lastSeen = lastSeen;
        }
    }
}
