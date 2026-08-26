
# React Native Design

Master React Native styling patterns, React Navigation, and Reanimated 3 to build performant, cross-platform mobile applications with native-quality user experiences.

## When to Use This Skill

- Building cross-platform mobile apps with React Native
- Implementing navigation with React Navigation 6+
- Creating performant animations with Reanimated 3
- Styling components with StyleSheet and styled-components
- Building responsive layouts for different screen sizes
- Implementing platform-specific designs (iOS/Android)
- Creating gesture-driven interactions with Gesture Handler
- Optimizing React Native performance

## Detailed section: Core Concepts

The detailed patterns follow below in this file.

## Quick Start Component

```typescript
import React from 'react';
import {
  View,
  Text,
  StyleSheet,
  Pressable,
  Image,
} from 'react-native';
import Animated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
} from 'react-native-reanimated';

interface ItemCardProps {
  title: string;
  subtitle: string;
  imageUrl: string;
  onPress: () => void;
}

const AnimatedPressable = Animated.createAnimatedComponent(Pressable);

export function ItemCard({ title, subtitle, imageUrl, onPress }: ItemCardProps) {
  const scale = useSharedValue(1);

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [{ scale: scale.value }],
  }));

  return (
    <AnimatedPressable
      style={[styles.card, animatedStyle]}
      onPress={onPress}
      onPressIn={() => { scale.value = withSpring(0.97); }}
      onPressOut={() => { scale.value = withSpring(1); }}
    >
      <Image source={{ uri: imageUrl }} style={styles.image} />
      <View style={styles.content}>
        <Text style={styles.title} numberOfLines={1}>
          {title}
        </Text>
        <Text style={styles.subtitle} numberOfLines={2}>
          {subtitle}
        </Text>
      </View>
    </AnimatedPressable>
  );
}

const styles = StyleSheet.create({
  card: {
    backgroundColor: '#ffffff',
    borderRadius: 16,
    overflow: 'hidden',
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.1,
    shadowRadius: 8,
    elevation: 4,
  },
  image: {
    width: '100%',
    height: 160,
    backgroundColor: '#f3f4f6',
  },
  content: {
    padding: 16,
    gap: 4,
  },
  title: {
    fontSize: 18,
    fontWeight: '600',
    color: '#1f2937',
  },
  subtitle: {
    fontSize: 14,
    color: '#6b7280',
    lineHeight: 20,
  },
});
```

## Best Practices

1. **Use TypeScript**: Define navigation and prop types for type safety
2. **Memoize Components**: Use `React.memo` and `useCallback` to prevent unnecessary rerenders
3. **Run Animations on UI Thread**: Use Reanimated worklets for 60fps animations
4. **Avoid Inline Styles**: Use StyleSheet.create for performance
5. **Handle Safe Areas**: Use `SafeAreaView` or `useSafeAreaInsets`
6. **Test on Real Devices**: Simulator/emulator performance differs from real devices
7. **Use FlatList for Lists**: Never use ScrollView with map for long lists
8. **Platform-Specific Code**: Use Platform.select for iOS/Android differences

## Common Issues

- **Gesture Conflicts**: Wrap gestures with `GestureDetector` and use `simultaneousHandlers`
- **Navigation Type Errors**: Define `ParamList` types for all navigators
- **Animation Jank**: Move animations to UI thread with `runOnUI` worklets
- **Memory Leaks**: Cancel animations and cleanup in useEffect
- **Font Loading**: Use `expo-font` or `react-native-asset` for custom fonts
- **Safe Area Issues**: Test on notched devices (iPhone, Android with cutouts)


---

## อ้างอิง: details

# react-native-design — detailed sections

## Core Concepts

### 1. StyleSheet and Styling

**Basic StyleSheet:**

```typescript
import { StyleSheet, View, Text } from 'react-native';

const styles = StyleSheet.create({
  container: {
    flex: 1,
    padding: 16,
    backgroundColor: '#ffffff',
  },
  title: {
    fontSize: 24,
    fontWeight: '600',
    color: '#1a1a1a',
    marginBottom: 8,
  },
  subtitle: {
    fontSize: 16,
    color: '#666666',
    lineHeight: 24,
  },
});

function Card() {
  return (
    <View style={styles.container}>
      <Text style={styles.title}>Title</Text>
      <Text style={styles.subtitle}>Subtitle text</Text>
    </View>
  );
}
```

**Dynamic Styles:**

```typescript
interface CardProps {
  variant: 'primary' | 'secondary';
  disabled?: boolean;
}

function Card({ variant, disabled }: CardProps) {
  return (
    <View style={[
      styles.card,
      variant === 'primary' ? styles.primary : styles.secondary,
      disabled && styles.disabled,
    ]}>
      <Text style={styles.text}>Content</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  card: {
    padding: 16,
    borderRadius: 12,
  },
  primary: {
    backgroundColor: '#6366f1',
  },
  secondary: {
    backgroundColor: '#f3f4f6',
    borderWidth: 1,
    borderColor: '#e5e7eb',
  },
  disabled: {
    opacity: 0.5,
  },
  text: {
    fontSize: 16,
  },
});
```

### 2. Flexbox Layout

**Row and Column Layouts:**

```typescript
const styles = StyleSheet.create({
  // Vertical stack (column)
  column: {
    flexDirection: "column",
    gap: 12,
  },
  // Horizontal stack (row)
  row: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
  },
  // Space between items
  spaceBetween: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
  },
  // Centered content
  centered: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
  },
  // Fill remaining space
  fill: {
    flex: 1,
  },
});
```

### 3. React Navigation Setup

**Stack Navigator:**

```typescript
import { NavigationContainer } from '@react-navigation/native';
import { createNativeStackNavigator } from '@react-navigation/native-stack';

type RootStackParamList = {
  Home: undefined;
  Detail: { itemId: string };
  Settings: undefined;
};

const Stack = createNativeStackNavigator<RootStackParamList>();

function AppNavigator() {
  return (
    <NavigationContainer>
      <Stack.Navigator
        initialRouteName="Home"
        screenOptions={{
          headerStyle: { backgroundColor: '#6366f1' },
          headerTintColor: '#ffffff',
          headerTitleStyle: { fontWeight: '600' },
        }}
      >
        <Stack.Screen
          name="Home"
          component={HomeScreen}
          options={{ title: 'Home' }}
        />
        <Stack.Screen
          name="Detail"
          component={DetailScreen}
          options={({ route }) => ({
            title: `Item ${route.params.itemId}`,
          })}
        />
        <Stack.Screen name="Settings" component={SettingsScreen} />
      </Stack.Navigator>
    </NavigationContainer>
  );
}
```

**Tab Navigator:**

```typescript
import { createBottomTabNavigator } from '@react-navigation/bottom-tabs';
import { Ionicons } from '@expo/vector-icons';

type TabParamList = {
  Home: undefined;
  Search: undefined;
  Profile: undefined;
};

const Tab = createBottomTabNavigator<TabParamList>();

function TabNavigator() {
  return (
    <Tab.Navigator
      screenOptions={({ route }) => ({
        tabBarIcon: ({ focused, color, size }) => {
          const icons: Record<string, keyof typeof Ionicons.glyphMap> = {
            Home: focused ? 'home' : 'home-outline',
            Search: focused ? 'search' : 'search-outline',
            Profile: focused ? 'person' : 'person-outline',
          };
          return <Ionicons name={icons[route.name]} size={size} color={color} />;
        },
        tabBarActiveTintColor: '#6366f1',
        tabBarInactiveTintColor: '#9ca3af',
      })}
    >
      <Tab.Screen name="Home" component={HomeScreen} />
      <Tab.Screen name="Search" component={SearchScreen} />
      <Tab.Screen name="Profile" component={ProfileScreen} />
    </Tab.Navigator>
  );
}
```

### 4. Reanimated 3 Basics

**Animated Values:**

```typescript
import Animated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
  withTiming,
} from 'react-native-reanimated';

function AnimatedBox() {
  const scale = useSharedValue(1);
  const opacity = useSharedValue(1);

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [{ scale: scale.value }],
    opacity: opacity.value,
  }));

  const handlePress = () => {
    scale.value = withSpring(1.2, {}, () => {
      scale.value = withSpring(1);
    });
  };

  return (
    <Pressable onPress={handlePress}>
      <Animated.View style={[styles.box, animatedStyle]} />
    </Pressable>
  );
}
```

**Gesture Handler Integration:**

```typescript
import { Gesture, GestureDetector } from 'react-native-gesture-handler';
import Animated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
} from 'react-native-reanimated';

function DraggableCard() {
  const translateX = useSharedValue(0);
  const translateY = useSharedValue(0);

  const gesture = Gesture.Pan()
    .onUpdate((event) => {
      translateX.value = event.translationX;
      translateY.value = event.translationY;
    })
    .onEnd(() => {
      translateX.value = withSpring(0);
      translateY.value = withSpring(0);
    });

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [
      { translateX: translateX.value },
      { translateY: translateY.value },
    ],
  }));

  return (
    <GestureDetector gesture={gesture}>
      <Animated.View style={[styles.card, animatedStyle]}>
        <Text>Drag me!</Text>
      </Animated.View>
    </GestureDetector>
  );
}
```

### 5. Platform-Specific Styling

```typescript
import { Platform, StyleSheet } from "react-native";

const styles = StyleSheet.create({
  container: {
    padding: 16,
    ...Platform.select({
      ios: {
        shadowColor: "#000",
        shadowOffset: { width: 0, height: 2 },
        shadowOpacity: 0.1,
        shadowRadius: 4,
      },
      android: {
        elevation: 4,
      },
    }),
  },
  text: {
    fontFamily: Platform.OS === "ios" ? "SF Pro Text" : "Roboto",
    fontSize: 16,
  },
});

// Platform-specific components
import { Platform } from "react-native";
const StatusBarHeight = Platform.OS === "ios" ? 44 : 0;
```


---

## อ้างอิง: navigation-patterns

# React Navigation Patterns

## Setup and Configuration

### Installation

```bash
# Core packages
npm install @react-navigation/native
npm install @react-navigation/native-stack
npm install @react-navigation/bottom-tabs

# Required peer dependencies
npm install react-native-screens react-native-safe-area-context
```

### Type-Safe Navigation Setup

```typescript
// navigation/types.ts
import { NativeStackScreenProps } from "@react-navigation/native-stack";
import { BottomTabScreenProps } from "@react-navigation/bottom-tabs";
import {
  CompositeScreenProps,
  NavigatorScreenParams,
} from "@react-navigation/native";

// Define param lists for each navigator
export type RootStackParamList = {
  Main: NavigatorScreenParams<MainTabParamList>;
  Modal: { title: string };
  Auth: NavigatorScreenParams<AuthStackParamList>;
};

export type MainTabParamList = {
  Home: undefined;
  Search: { query?: string };
  Profile: { userId: string };
};

export type AuthStackParamList = {
  Login: undefined;
  Register: undefined;
  ForgotPassword: { email?: string };
};

// Screen props helpers
export type RootStackScreenProps<T extends keyof RootStackParamList> =
  NativeStackScreenProps<RootStackParamList, T>;

export type MainTabScreenProps<T extends keyof MainTabParamList> =
  CompositeScreenProps<
    BottomTabScreenProps<MainTabParamList, T>,
    RootStackScreenProps<keyof RootStackParamList>
  >;

// Global type declaration
declare global {
  namespace ReactNavigation {
    interface RootParamList extends RootStackParamList {}
  }
}
```

### Navigation Hooks

```typescript
// hooks/useAppNavigation.ts
import { useNavigation, useRoute, RouteProp } from "@react-navigation/native";
import { NativeStackNavigationProp } from "@react-navigation/native-stack";
import { RootStackParamList } from "./types";

export function useAppNavigation() {
  return useNavigation<NativeStackNavigationProp<RootStackParamList>>();
}

export function useTypedRoute<T extends keyof RootStackParamList>() {
  return useRoute<RouteProp<RootStackParamList, T>>();
}
```

## Stack Navigation

### Basic Stack Navigator

```typescript
import { createNativeStackNavigator } from '@react-navigation/native-stack';
import { RootStackParamList } from './types';

const Stack = createNativeStackNavigator<RootStackParamList>();

function RootNavigator() {
  return (
    <Stack.Navigator
      initialRouteName="Main"
      screenOptions={{
        headerStyle: { backgroundColor: '#6366f1' },
        headerTintColor: '#ffffff',
        headerTitleStyle: { fontWeight: '600' },
        headerBackTitleVisible: false,
        animation: 'slide_from_right',
      }}
    >
      <Stack.Screen
        name="Main"
        component={MainTabNavigator}
        options={{ headerShown: false }}
      />
      <Stack.Screen
        name="Modal"
        component={ModalScreen}
        options={{
          presentation: 'modal',
          animation: 'slide_from_bottom',
        }}
      />
      <Stack.Group screenOptions={{ presentation: 'fullScreenModal' }}>
        <Stack.Screen
          name="Auth"
          component={AuthNavigator}
          options={{ headerShown: false }}
        />
      </Stack.Group>
    </Stack.Navigator>
  );
}
```

### Screen with Dynamic Options

```typescript
function DetailScreen({ route, navigation }: DetailScreenProps) {
  const { itemId } = route.params;
  const [item, setItem] = useState<Item | null>(null);

  useEffect(() => {
    // Update header when data loads
    if (item) {
      navigation.setOptions({
        title: item.title,
        headerRight: () => (
          <TouchableOpacity onPress={() => shareItem(item)}>
            <Ionicons name="share-outline" size={24} color="#ffffff" />
          </TouchableOpacity>
        ),
      });
    }
  }, [item, navigation]);

  // Prevent going back with unsaved changes
  useEffect(() => {
    const unsubscribe = navigation.addListener('beforeRemove', (e) => {
      if (!hasUnsavedChanges) return;

      e.preventDefault();
      Alert.alert(
        'Discard changes?',
        'You have unsaved changes. Are you sure you want to leave?',
        [
          { text: "Don't leave", style: 'cancel' },
          {
            text: 'Discard',
            style: 'destructive',
            onPress: () => navigation.dispatch(e.data.action),
          },
        ]
      );
    });

    return unsubscribe;
  }, [navigation, hasUnsavedChanges]);

  return <View>{/* Content */}</View>;
}
```

## Tab Navigation

### Bottom Tab Navigator

```typescript
import { createBottomTabNavigator } from '@react-navigation/bottom-tabs';
import { MainTabParamList } from './types';
import { Ionicons } from '@expo/vector-icons';

const Tab = createBottomTabNavigator<MainTabParamList>();

function MainTabNavigator() {
  return (
    <Tab.Navigator
      screenOptions={({ route }) => ({
        tabBarIcon: ({ focused, color, size }) => {
          const icons: Record<keyof MainTabParamList, string> = {
            Home: focused ? 'home' : 'home-outline',
            Search: focused ? 'search' : 'search-outline',
            Profile: focused ? 'person' : 'person-outline',
          };
          return (
            <Ionicons
              name={icons[route.name] as any}
              size={size}
              color={color}
            />
          );
        },
        tabBarActiveTintColor: '#6366f1',
        tabBarInactiveTintColor: '#9ca3af',
        tabBarStyle: {
          backgroundColor: '#ffffff',
          borderTopWidth: 1,
          borderTopColor: '#e5e7eb',
          paddingBottom: 8,
          paddingTop: 8,
          height: 60,
        },
        tabBarLabelStyle: {
          fontSize: 12,
          fontWeight: '500',
        },
        headerStyle: { backgroundColor: '#ffffff' },
        headerTitleStyle: { fontWeight: '600' },
      })}
    >
      <Tab.Screen
        name="Home"
        component={HomeScreen}
        options={{
          tabBarLabel: 'Home',
          tabBarBadge: 3,
        }}
      />
      <Tab.Screen
        name="Search"
        component={SearchScreen}
        options={{ tabBarLabel: 'Search' }}
      />
      <Tab.Screen
        name="Profile"
        component={ProfileScreen}
        options={{ tabBarLabel: 'Profile' }}
      />
    </Tab.Navigator>
  );
}
```

### Custom Tab Bar

```typescript
import { View, Pressable, StyleSheet } from 'react-native';
import { BottomTabBarProps } from '@react-navigation/bottom-tabs';
import Animated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
} from 'react-native-reanimated';

function CustomTabBar({ state, descriptors, navigation }: BottomTabBarProps) {
  return (
    <View style={styles.tabBar}>
      {state.routes.map((route, index) => {
        const { options } = descriptors[route.key];
        const label = options.tabBarLabel ?? route.name;
        const isFocused = state.index === index;

        const onPress = () => {
          const event = navigation.emit({
            type: 'tabPress',
            target: route.key,
            canPreventDefault: true,
          });

          if (!isFocused && !event.defaultPrevented) {
            navigation.navigate(route.name);
          }
        };

        return (
          <TabBarButton
            key={route.key}
            label={label as string}
            isFocused={isFocused}
            onPress={onPress}
            icon={options.tabBarIcon}
          />
        );
      })}
    </View>
  );
}

function TabBarButton({ label, isFocused, onPress, icon }) {
  const scale = useSharedValue(1);

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [{ scale: scale.value }],
  }));

  return (
    <Pressable
      onPress={onPress}
      onPressIn={() => { scale.value = withSpring(0.9); }}
      onPressOut={() => { scale.value = withSpring(1); }}
      style={styles.tabButton}
    >
      <Animated.View style={animatedStyle}>
        {icon?.({
          focused: isFocused,
          color: isFocused ? '#6366f1' : '#9ca3af',
          size: 24,
        })}
        <Text
          style={[
            styles.tabLabel,
            { color: isFocused ? '#6366f1' : '#9ca3af' },
          ]}
        >
          {label}
        </Text>
      </Animated.View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  tabBar: {
    flexDirection: 'row',
    backgroundColor: '#ffffff',
    paddingBottom: 20,
    paddingTop: 12,
    borderTopWidth: 1,
    borderTopColor: '#e5e7eb',
  },
  tabButton: {
    flex: 1,
    alignItems: 'center',
  },
  tabLabel: {
    fontSize: 12,
    marginTop: 4,
    fontWeight: '500',
  },
});

// Usage
<Tab.Navigator tabBar={(props) => <CustomTabBar {...props} />}>
  {/* screens */}
</Tab.Navigator>
```

## Drawer Navigation

```typescript
import {
  createDrawerNavigator,
  DrawerContentScrollView,
  DrawerItemList,
  DrawerContentComponentProps,
} from '@react-navigation/drawer';

const Drawer = createDrawerNavigator();

function CustomDrawerContent(props: DrawerContentComponentProps) {
  return (
    <DrawerContentScrollView {...props}>
      <View style={styles.drawerHeader}>
        <Image source={{ uri: user.avatar }} style={styles.avatar} />
        <Text style={styles.userName}>{user.name}</Text>
        <Text style={styles.userEmail}>{user.email}</Text>
      </View>
      <DrawerItemList {...props} />
      <View style={styles.drawerFooter}>
        <TouchableOpacity
          onPress={handleLogout}
          style={styles.logoutButton}
        >
          <Ionicons name="log-out-outline" size={24} color="#ef4444" />
          <Text style={styles.logoutText}>Log Out</Text>
        </TouchableOpacity>
      </View>
    </DrawerContentScrollView>
  );
}

function DrawerNavigator() {
  return (
    <Drawer.Navigator
      drawerContent={(props) => <CustomDrawerContent {...props} />}
      screenOptions={{
        drawerActiveBackgroundColor: '#ede9fe',
        drawerActiveTintColor: '#6366f1',
        drawerInactiveTintColor: '#4b5563',
        drawerLabelStyle: { marginLeft: -20, fontSize: 15, fontWeight: '500' },
        drawerStyle: { width: 280 },
      }}
    >
      <Drawer.Screen
        name="Home"
        component={HomeScreen}
        options={{
          drawerIcon: ({ color }) => (
            <Ionicons name="home-outline" size={22} color={color} />
          ),
        }}
      />
      <Drawer.Screen
        name="Settings"
        component={SettingsScreen}
        options={{
          drawerIcon: ({ color }) => (
            <Ionicons name="settings-outline" size={22} color={color} />
          ),
        }}
      />
    </Drawer.Navigator>
  );
}
```

## Deep Linking

### Configuration

```typescript
// navigation/linking.ts
import { LinkingOptions } from '@react-navigation/native';
import { RootStackParamList } from './types';

export const linking: LinkingOptions<RootStackParamList> = {
  prefixes: ['myapp://', 'https://myapp.com'],
  config: {
    screens: {
      Main: {
        screens: {
          Home: 'home',
          Search: 'search',
          Profile: 'profile/:userId',
        },
      },
      Modal: 'modal/:title',
      Auth: {
        screens: {
          Login: 'login',
          Register: 'register',
          ForgotPassword: 'forgot-password',
        },
      },
    },
  },
  // Custom URL parsing
  getStateFromPath: (path, config) => {
    // Handle custom URL patterns
    return getStateFromPath(path, config);
  },
};

// App.tsx
function App() {
  return (
    <NavigationContainer linking={linking} fallback={<LoadingScreen />}>
      <RootNavigator />
    </NavigationContainer>
  );
}
```

### Handling Deep Links

```typescript
import { useEffect } from "react";
import { Linking } from "react-native";
import { useNavigation } from "@react-navigation/native";

function useDeepLinkHandler() {
  const navigation = useNavigation();

  useEffect(() => {
    // Handle initial URL
    const handleInitialUrl = async () => {
      const url = await Linking.getInitialURL();
      if (url) {
        handleDeepLink(url);
      }
    };

    // Handle URL changes
    const subscription = Linking.addEventListener("url", ({ url }) => {
      handleDeepLink(url);
    });

    handleInitialUrl();

    return () => subscription.remove();
  }, []);

  const handleDeepLink = (url: string) => {
    // Parse URL and navigate
    const route = parseUrl(url);
    if (route) {
      navigation.navigate(route.name, route.params);
    }
  };
}
```

## Navigation State Management

### Auth Flow

```typescript
import { createContext, useContext, useState, useEffect } from 'react';

interface AuthContextType {
  user: User | null;
  isLoading: boolean;
  signIn: (email: string, password: string) => Promise<void>;
  signOut: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType>(null!);

function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    // Check for existing session
    checkAuthState();
  }, []);

  const checkAuthState = async () => {
    try {
      const token = await AsyncStorage.getItem('token');
      if (token) {
        const user = await fetchUser(token);
        setUser(user);
      }
    } catch (error) {
      console.error(error);
    } finally {
      setIsLoading(false);
    }
  };

  const signIn = async (email: string, password: string) => {
    const { user, token } = await loginApi(email, password);
    await AsyncStorage.setItem('token', token);
    setUser(user);
  };

  const signOut = async () => {
    await AsyncStorage.removeItem('token');
    setUser(null);
  };

  return (
    <AuthContext.Provider value={{ user, isLoading, signIn, signOut }}>
      {children}
    </AuthContext.Provider>
  );
}

function RootNavigator() {
  const { user, isLoading } = useAuth();

  if (isLoading) {
    return <SplashScreen />;
  }

  return (
    <Stack.Navigator screenOptions={{ headerShown: false }}>
      {user ? (
        <Stack.Screen name="Main" component={MainNavigator} />
      ) : (
        <Stack.Screen name="Auth" component={AuthNavigator} />
      )}
    </Stack.Navigator>
  );
}
```

### Navigation State Persistence

```typescript
import AsyncStorage from '@react-native-async-storage/async-storage';
import { NavigationContainer, NavigationState } from '@react-navigation/native';

const PERSISTENCE_KEY = 'NAVIGATION_STATE';

function App() {
  const [isReady, setIsReady] = useState(false);
  const [initialState, setInitialState] = useState<NavigationState | undefined>();

  useEffect(() => {
    const restoreState = async () => {
      try {
        const savedState = await AsyncStorage.getItem(PERSISTENCE_KEY);
        if (savedState) {
          setInitialState(JSON.parse(savedState));
        }
      } catch (e) {
        console.error('Failed to restore navigation state:', e);
      } finally {
        setIsReady(true);
      }
    };

    if (!isReady) {
      restoreState();
    }
  }, [isReady]);

  if (!isReady) {
    return <SplashScreen />;
  }

  return (
    <NavigationContainer
      initialState={initialState}
      onStateChange={(state) => {
        AsyncStorage.setItem(PERSISTENCE_KEY, JSON.stringify(state));
      }}
    >
      <RootNavigator />
    </NavigationContainer>
  );
}
```

## Screen Transitions

### Custom Animations

```typescript
import { TransitionPresets } from '@react-navigation/native-stack';

<Stack.Navigator
  screenOptions={{
    ...TransitionPresets.SlideFromRightIOS,
    gestureEnabled: true,
    gestureDirection: 'horizontal',
  }}
>
  {/* Standard slide transition */}
  <Stack.Screen name="List" component={ListScreen} />

  {/* Modal with custom animation */}
  <Stack.Screen
    name="Modal"
    component={ModalScreen}
    options={{
      presentation: 'transparentModal',
      animation: 'fade',
      cardOverlayEnabled: true,
    }}
  />

  {/* Full screen modal */}
  <Stack.Screen
    name="FullScreenModal"
    component={FullScreenModalScreen}
    options={{
      presentation: 'fullScreenModal',
      animation: 'slide_from_bottom',
    }}
  />
</Stack.Navigator>
```

### Shared Element Transitions

```typescript
import { SharedElement } from 'react-navigation-shared-element';
import { createSharedElementStackNavigator } from 'react-navigation-shared-element';

const Stack = createSharedElementStackNavigator();

function ListScreen({ navigation }) {
  return (
    <FlatList
      data={items}
      renderItem={({ item }) => (
        <Pressable onPress={() => navigation.navigate('Detail', { item })}>
          <SharedElement id={`item.${item.id}.photo`}>
            <Image source={{ uri: item.imageUrl }} style={styles.image} />
          </SharedElement>
          <SharedElement id={`item.${item.id}.title`}>
            <Text style={styles.title}>{item.title}</Text>
          </SharedElement>
        </Pressable>
      )}
    />
  );
}

function DetailScreen({ route }) {
  const { item } = route.params;

  return (
    <View>
      <SharedElement id={`item.${item.id}.photo`}>
        <Image source={{ uri: item.imageUrl }} style={styles.heroImage} />
      </SharedElement>
      <SharedElement id={`item.${item.id}.title`}>
        <Text style={styles.title}>{item.title}</Text>
      </SharedElement>
    </View>
  );
}

// Navigator configuration
<Stack.Navigator>
  <Stack.Screen name="List" component={ListScreen} />
  <Stack.Screen
    name="Detail"
    component={DetailScreen}
    sharedElements={(route) => {
      const { item } = route.params;
      return [
        { id: `item.${item.id}.photo`, animation: 'move' },
        { id: `item.${item.id}.title`, animation: 'fade' },
      ];
    }}
  />
</Stack.Navigator>
```

## Header Customization

### Custom Header Component

```typescript
import { getHeaderTitle } from '@react-navigation/elements';
import { NativeStackHeaderProps } from '@react-navigation/native-stack';

function CustomHeader({ navigation, route, options, back }: NativeStackHeaderProps) {
  const title = getHeaderTitle(options, route.name);

  return (
    <View style={styles.header}>
      {back && (
        <TouchableOpacity
          onPress={navigation.goBack}
          style={styles.backButton}
        >
          <Ionicons name="arrow-back" size={24} color="#1f2937" />
        </TouchableOpacity>
      )}
      <Text style={styles.title}>{title}</Text>
      {options.headerRight && (
        <View style={styles.rightActions}>
          {options.headerRight({ canGoBack: !!back })}
        </View>
      )}
    </View>
  );
}

// Usage
<Stack.Navigator
  screenOptions={{
    header: (props) => <CustomHeader {...props} />,
  }}
>
  {/* screens */}
</Stack.Navigator>
```

### Collapsible Header

```typescript
import Animated, {
  useSharedValue,
  useAnimatedScrollHandler,
  useAnimatedStyle,
  interpolate,
  Extrapolation,
} from 'react-native-reanimated';

const HEADER_HEIGHT = 200;
const COLLAPSED_HEIGHT = 60;

function CollapsibleHeaderScreen() {
  const scrollY = useSharedValue(0);

  const scrollHandler = useAnimatedScrollHandler({
    onScroll: (event) => {
      scrollY.value = event.contentOffset.y;
    },
  });

  const headerStyle = useAnimatedStyle(() => {
    const height = interpolate(
      scrollY.value,
      [0, HEADER_HEIGHT - COLLAPSED_HEIGHT],
      [HEADER_HEIGHT, COLLAPSED_HEIGHT],
      Extrapolation.CLAMP
    );

    return { height };
  });

  const titleStyle = useAnimatedStyle(() => {
    const fontSize = interpolate(
      scrollY.value,
      [0, HEADER_HEIGHT - COLLAPSED_HEIGHT],
      [32, 18],
      Extrapolation.CLAMP
    );

    return { fontSize };
  });

  return (
    <View style={styles.container}>
      <Animated.View style={[styles.header, headerStyle]}>
        <Animated.Text style={[styles.title, titleStyle]}>
          Title
        </Animated.Text>
      </Animated.View>

      <Animated.ScrollView
        onScroll={scrollHandler}
        scrollEventThrottle={16}
        contentContainerStyle={{ paddingTop: HEADER_HEIGHT }}
      >
        {/* Content */}
      </Animated.ScrollView>
    </View>
  );
}
```


---

## อ้างอิง: reanimated-patterns

# React Native Reanimated 3 Patterns

## Core Concepts

### Shared Values and Animated Styles

```typescript
import Animated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
  withTiming,
  withDelay,
  withSequence,
  withRepeat,
  Easing,
} from 'react-native-reanimated';

function BasicAnimations() {
  // Shared value - can be modified from JS or UI thread
  const opacity = useSharedValue(0);
  const scale = useSharedValue(1);
  const rotation = useSharedValue(0);

  // Animated style - runs on UI thread
  const animatedStyle = useAnimatedStyle(() => ({
    opacity: opacity.value,
    transform: [
      { scale: scale.value },
      { rotate: `${rotation.value}deg` },
    ],
  }));

  const animate = () => {
    // Spring animation
    scale.value = withSpring(1.2, {
      damping: 10,
      stiffness: 100,
    });

    // Timing animation with easing
    opacity.value = withTiming(1, {
      duration: 500,
      easing: Easing.bezier(0.25, 0.1, 0.25, 1),
    });

    // Sequence of animations
    rotation.value = withSequence(
      withTiming(15, { duration: 100 }),
      withTiming(-15, { duration: 100 }),
      withTiming(0, { duration: 100 })
    );
  };

  return (
    <Animated.View style={[styles.box, animatedStyle]} />
  );
}
```

### Animation Callbacks

```typescript
import { runOnJS, runOnUI } from 'react-native-reanimated';

function AnimationWithCallbacks() {
  const translateX = useSharedValue(0);
  const [status, setStatus] = useState('idle');

  const updateStatus = (newStatus: string) => {
    setStatus(newStatus);
  };

  const animate = () => {
    translateX.value = withTiming(
      200,
      { duration: 1000 },
      (finished) => {
        'worklet';
        if (finished) {
          // Call JS function from worklet
          runOnJS(updateStatus)('completed');
        }
      }
    );
  };

  return (
    <Animated.View
      style={[
        styles.box,
        useAnimatedStyle(() => ({
          transform: [{ translateX: translateX.value }],
        })),
      ]}
    />
  );
}
```

## Gesture Handler Integration

### Pan Gesture

```typescript
import { Gesture, GestureDetector } from 'react-native-gesture-handler';
import Animated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
  clamp,
} from 'react-native-reanimated';

function DraggableBox() {
  const translateX = useSharedValue(0);
  const translateY = useSharedValue(0);
  const context = useSharedValue({ x: 0, y: 0 });

  const panGesture = Gesture.Pan()
    .onStart(() => {
      context.value = { x: translateX.value, y: translateY.value };
    })
    .onUpdate((event) => {
      translateX.value = event.translationX + context.value.x;
      translateY.value = event.translationY + context.value.y;
    })
    .onEnd((event) => {
      // Apply velocity decay
      translateX.value = withSpring(
        clamp(translateX.value + event.velocityX / 10, -100, 100)
      );
      translateY.value = withSpring(
        clamp(translateY.value + event.velocityY / 10, -100, 100)
      );
    });

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [
      { translateX: translateX.value },
      { translateY: translateY.value },
    ],
  }));

  return (
    <GestureDetector gesture={panGesture}>
      <Animated.View style={[styles.box, animatedStyle]} />
    </GestureDetector>
  );
}
```

### Pinch and Rotate Gestures

```typescript
function ZoomableImage() {
  const scale = useSharedValue(1);
  const rotation = useSharedValue(0);
  const savedScale = useSharedValue(1);
  const savedRotation = useSharedValue(0);

  const pinchGesture = Gesture.Pinch()
    .onUpdate((event) => {
      scale.value = savedScale.value * event.scale;
    })
    .onEnd(() => {
      savedScale.value = scale.value;
      // Snap back if too small
      if (scale.value < 1) {
        scale.value = withSpring(1);
        savedScale.value = 1;
      }
    });

  const rotateGesture = Gesture.Rotation()
    .onUpdate((event) => {
      rotation.value = savedRotation.value + event.rotation;
    })
    .onEnd(() => {
      savedRotation.value = rotation.value;
    });

  const composed = Gesture.Simultaneous(pinchGesture, rotateGesture);

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [
      { scale: scale.value },
      { rotate: `${rotation.value}rad` },
    ],
  }));

  return (
    <GestureDetector gesture={composed}>
      <Animated.Image
        source={{ uri: imageUrl }}
        style={[styles.image, animatedStyle]}
      />
    </GestureDetector>
  );
}
```

### Tap Gesture with Feedback

```typescript
function TappableCard({ onPress, children }: TappableCardProps) {
  const scale = useSharedValue(1);
  const opacity = useSharedValue(1);

  const tapGesture = Gesture.Tap()
    .onBegin(() => {
      scale.value = withSpring(0.97);
      opacity.value = withTiming(0.8, { duration: 100 });
    })
    .onFinalize(() => {
      scale.value = withSpring(1);
      opacity.value = withTiming(1, { duration: 100 });
    })
    .onEnd(() => {
      runOnJS(onPress)();
    });

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [{ scale: scale.value }],
    opacity: opacity.value,
  }));

  return (
    <GestureDetector gesture={tapGesture}>
      <Animated.View style={[styles.card, animatedStyle]}>
        {children}
      </Animated.View>
    </GestureDetector>
  );
}
```

## Common Animation Patterns

### Fade In/Out

```typescript
function FadeInView({ visible, children }: FadeInViewProps) {
  const opacity = useSharedValue(0);

  useEffect(() => {
    opacity.value = withTiming(visible ? 1 : 0, { duration: 300 });
  }, [visible]);

  const animatedStyle = useAnimatedStyle(() => ({
    opacity: opacity.value,
    display: opacity.value === 0 ? 'none' : 'flex',
  }));

  return (
    <Animated.View style={animatedStyle}>
      {children}
    </Animated.View>
  );
}
```

### Slide In/Out

```typescript
function SlideInView({ visible, direction = 'right', children }) {
  const translateX = useSharedValue(direction === 'right' ? 100 : -100);
  const opacity = useSharedValue(0);

  useEffect(() => {
    if (visible) {
      translateX.value = withSpring(0);
      opacity.value = withTiming(1);
    } else {
      translateX.value = withSpring(direction === 'right' ? 100 : -100);
      opacity.value = withTiming(0);
    }
  }, [visible, direction]);

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [{ translateX: translateX.value }],
    opacity: opacity.value,
  }));

  return (
    <Animated.View style={animatedStyle}>
      {children}
    </Animated.View>
  );
}
```

### Staggered List Animation

```typescript
function StaggeredList({ items }: { items: Item[] }) {
  return (
    <FlatList
      data={items}
      keyExtractor={(item) => item.id}
      renderItem={({ item, index }) => (
        <StaggeredItem item={item} index={index} />
      )}
    />
  );
}

function StaggeredItem({ item, index }: { item: Item; index: number }) {
  const translateY = useSharedValue(50);
  const opacity = useSharedValue(0);

  useEffect(() => {
    translateY.value = withDelay(
      index * 100,
      withSpring(0, { damping: 15 })
    );
    opacity.value = withDelay(
      index * 100,
      withTiming(1, { duration: 300 })
    );
  }, [index]);

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [{ translateY: translateY.value }],
    opacity: opacity.value,
  }));

  return (
    <Animated.View style={[styles.listItem, animatedStyle]}>
      <Text>{item.title}</Text>
    </Animated.View>
  );
}
```

### Pulse Animation

```typescript
function PulseView({ children }: { children: React.ReactNode }) {
  const scale = useSharedValue(1);

  useEffect(() => {
    scale.value = withRepeat(
      withSequence(
        withTiming(1.05, { duration: 500 }),
        withTiming(1, { duration: 500 })
      ),
      -1, // infinite
      false // no reverse
    );

    return () => {
      cancelAnimation(scale);
    };
  }, []);

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [{ scale: scale.value }],
  }));

  return (
    <Animated.View style={animatedStyle}>
      {children}
    </Animated.View>
  );
}
```

### Shake Animation

```typescript
function ShakeView({ trigger, children }) {
  const translateX = useSharedValue(0);

  useEffect(() => {
    if (trigger) {
      translateX.value = withSequence(
        withTiming(-10, { duration: 50 }),
        withTiming(10, { duration: 50 }),
        withTiming(-10, { duration: 50 }),
        withTiming(10, { duration: 50 }),
        withTiming(0, { duration: 50 })
      );
    }
  }, [trigger]);

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [{ translateX: translateX.value }],
  }));

  return (
    <Animated.View style={animatedStyle}>
      {children}
    </Animated.View>
  );
}
```

## Advanced Patterns

### Interpolation

```typescript
import { interpolate, Extrapolation } from 'react-native-reanimated';

function ParallaxHeader() {
  const scrollY = useSharedValue(0);

  const headerStyle = useAnimatedStyle(() => {
    const height = interpolate(
      scrollY.value,
      [0, 200],
      [300, 100],
      Extrapolation.CLAMP
    );

    const opacity = interpolate(
      scrollY.value,
      [0, 150, 200],
      [1, 0.5, 0],
      Extrapolation.CLAMP
    );

    const translateY = interpolate(
      scrollY.value,
      [0, 200],
      [0, -50],
      Extrapolation.CLAMP
    );

    return {
      height,
      opacity,
      transform: [{ translateY }],
    };
  });

  return (
    <View style={styles.container}>
      <Animated.View style={[styles.header, headerStyle]}>
        <Text style={styles.headerTitle}>Header</Text>
      </Animated.View>
      <Animated.ScrollView
        onScroll={useAnimatedScrollHandler({
          onScroll: (event) => {
            scrollY.value = event.contentOffset.y;
          },
        })}
        scrollEventThrottle={16}
      >
        {/* Content */}
      </Animated.ScrollView>
    </View>
  );
}
```

### Color Interpolation

```typescript
import { interpolateColor } from 'react-native-reanimated';

function ColorTransition() {
  const progress = useSharedValue(0);

  const animatedStyle = useAnimatedStyle(() => {
    const backgroundColor = interpolateColor(
      progress.value,
      [0, 0.5, 1],
      ['#6366f1', '#8b5cf6', '#ec4899']
    );

    return { backgroundColor };
  });

  const toggleColor = () => {
    progress.value = withTiming(progress.value === 0 ? 1 : 0, {
      duration: 1000,
    });
  };

  return (
    <Pressable onPress={toggleColor}>
      <Animated.View style={[styles.box, animatedStyle]} />
    </Pressable>
  );
}
```

### Derived Values

```typescript
import { useDerivedValue } from 'react-native-reanimated';

function DerivedValueExample() {
  const x = useSharedValue(0);
  const y = useSharedValue(0);

  // Derived value computed from other shared values
  const distance = useDerivedValue(() => {
    return Math.sqrt(x.value ** 2 + y.value ** 2);
  });

  const angle = useDerivedValue(() => {
    return Math.atan2(y.value, x.value) * (180 / Math.PI);
  });

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [
      { translateX: x.value },
      { translateY: y.value },
      { rotate: `${angle.value}deg` },
    ],
    opacity: interpolate(distance.value, [0, 200], [1, 0.5]),
  }));

  return (
    <Animated.View style={[styles.box, animatedStyle]} />
  );
}
```

### Layout Animations

```typescript
import Animated, {
  Layout,
  FadeIn,
  FadeOut,
  SlideInLeft,
  SlideOutRight,
  ZoomIn,
  ZoomOut,
} from 'react-native-reanimated';

function AnimatedList() {
  const [items, setItems] = useState([1, 2, 3, 4, 5]);

  const addItem = () => {
    setItems([...items, items.length + 1]);
  };

  const removeItem = (id: number) => {
    setItems(items.filter((item) => item !== id));
  };

  return (
    <View style={styles.container}>
      <Button title="Add Item" onPress={addItem} />
      {items.map((item) => (
        <Animated.View
          key={item}
          style={styles.item}
          entering={FadeIn.duration(300).springify()}
          exiting={SlideOutRight.duration(300)}
          layout={Layout.springify()}
        >
          <Text>Item {item}</Text>
          <Pressable onPress={() => removeItem(item)}>
            <Text>Remove</Text>
          </Pressable>
        </Animated.View>
      ))}
    </View>
  );
}
```

### Swipeable Card

```typescript
function SwipeableCard({ onSwipeLeft, onSwipeRight }) {
  const translateX = useSharedValue(0);
  const rotateZ = useSharedValue(0);
  const context = useSharedValue({ x: 0 });

  const SWIPE_THRESHOLD = 120;

  const panGesture = Gesture.Pan()
    .onStart(() => {
      context.value = { x: translateX.value };
    })
    .onUpdate((event) => {
      translateX.value = event.translationX + context.value.x;
      rotateZ.value = interpolate(
        translateX.value,
        [-200, 0, 200],
        [-15, 0, 15]
      );
    })
    .onEnd((event) => {
      if (translateX.value > SWIPE_THRESHOLD) {
        translateX.value = withTiming(500, { duration: 200 }, () => {
          runOnJS(onSwipeRight)();
        });
      } else if (translateX.value < -SWIPE_THRESHOLD) {
        translateX.value = withTiming(-500, { duration: 200 }, () => {
          runOnJS(onSwipeLeft)();
        });
      } else {
        translateX.value = withSpring(0);
        rotateZ.value = withSpring(0);
      }
    });

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [
      { translateX: translateX.value },
      { rotate: `${rotateZ.value}deg` },
    ],
  }));

  const leftIndicatorStyle = useAnimatedStyle(() => ({
    opacity: interpolate(
      translateX.value,
      [0, SWIPE_THRESHOLD],
      [0, 1],
      Extrapolation.CLAMP
    ),
  }));

  const rightIndicatorStyle = useAnimatedStyle(() => ({
    opacity: interpolate(
      translateX.value,
      [-SWIPE_THRESHOLD, 0],
      [1, 0],
      Extrapolation.CLAMP
    ),
  }));

  return (
    <GestureDetector gesture={panGesture}>
      <Animated.View style={[styles.card, animatedStyle]}>
        <Animated.View style={[styles.likeIndicator, leftIndicatorStyle]}>
          <Text>LIKE</Text>
        </Animated.View>
        <Animated.View style={[styles.nopeIndicator, rightIndicatorStyle]}>
          <Text>NOPE</Text>
        </Animated.View>
        {/* Card content */}
      </Animated.View>
    </GestureDetector>
  );
}
```

### Bottom Sheet

```typescript
const MAX_TRANSLATE_Y = -SCREEN_HEIGHT + 50;
const MIN_TRANSLATE_Y = 0;
const SNAP_POINTS = [-SCREEN_HEIGHT * 0.5, -SCREEN_HEIGHT * 0.25, 0];

function BottomSheet({ children }) {
  const translateY = useSharedValue(0);
  const context = useSharedValue({ y: 0 });

  const panGesture = Gesture.Pan()
    .onStart(() => {
      context.value = { y: translateY.value };
    })
    .onUpdate((event) => {
      translateY.value = clamp(
        context.value.y + event.translationY,
        MAX_TRANSLATE_Y,
        MIN_TRANSLATE_Y
      );
    })
    .onEnd((event) => {
      // Find closest snap point
      const destination = SNAP_POINTS.reduce((prev, curr) =>
        Math.abs(curr - translateY.value) < Math.abs(prev - translateY.value)
          ? curr
          : prev
      );

      translateY.value = withSpring(destination, {
        damping: 50,
        stiffness: 300,
      });
    });

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [{ translateY: translateY.value }],
  }));

  const backdropStyle = useAnimatedStyle(() => ({
    opacity: interpolate(
      translateY.value,
      [MIN_TRANSLATE_Y, MAX_TRANSLATE_Y],
      [0, 0.5]
    ),
    pointerEvents: translateY.value < -50 ? 'auto' : 'none',
  }));

  return (
    <>
      <Animated.View style={[styles.backdrop, backdropStyle]} />
      <GestureDetector gesture={panGesture}>
        <Animated.View style={[styles.bottomSheet, animatedStyle]}>
          <View style={styles.handle} />
          {children}
        </Animated.View>
      </GestureDetector>
    </>
  );
}
```

## Performance Tips

### Memoization

```typescript
// Memoize animated style when dependencies don't change
const animatedStyle = useAnimatedStyle(
  () => ({
    transform: [{ translateX: translateX.value }],
  }),
  [],
); // Empty deps if no external dependencies

// Use useMemo for complex calculations outside worklets
const threshold = useMemo(() => calculateThreshold(screenWidth), [screenWidth]);
```

### Worklet Best Practices

```typescript
// Do: Keep worklets simple
const simpleWorklet = () => {
  "worklet";
  return scale.value * 2;
};

// Don't: Complex logic in worklets
// Move complex logic to JS with runOnJS

// Do: Use runOnJS for callbacks
const onComplete = () => {
  setIsAnimating(false);
};

opacity.value = withTiming(1, {}, (finished) => {
  "worklet";
  if (finished) {
    runOnJS(onComplete)();
  }
});
```

### Cancel Animations

```typescript
import { cancelAnimation } from 'react-native-reanimated';

function AnimatedComponent() {
  const translateX = useSharedValue(0);

  useEffect(() => {
    // Start animation
    translateX.value = withRepeat(
      withTiming(100, { duration: 1000 }),
      -1,
      true
    );

    // Cleanup: cancel animation on unmount
    return () => {
      cancelAnimation(translateX);
    };
  }, []);

  return <Animated.View style={animatedStyle} />;
}
```


---

## อ้างอิง: styling-patterns

# React Native Styling Patterns

## StyleSheet Fundamentals

### Creating Styles

```typescript
import { StyleSheet, ViewStyle, TextStyle, ImageStyle } from "react-native";

// Typed styles for better IDE support
interface Styles {
  container: ViewStyle;
  title: TextStyle;
  image: ImageStyle;
}

const styles = StyleSheet.create<Styles>({
  container: {
    flex: 1,
    padding: 16,
    backgroundColor: "#ffffff",
  },
  title: {
    fontSize: 24,
    fontWeight: "700",
    color: "#1f2937",
  },
  image: {
    width: 100,
    height: 100,
    borderRadius: 8,
  },
});
```

### Combining Styles

```typescript
import { StyleProp, ViewStyle } from 'react-native';

interface BoxProps {
  style?: StyleProp<ViewStyle>;
  variant?: 'default' | 'primary' | 'danger';
}

function Box({ style, variant = 'default' }: BoxProps) {
  return (
    <View
      style={[
        styles.base,
        variant === 'primary' && styles.primary,
        variant === 'danger' && styles.danger,
        style, // Allow external style overrides
      ]}
    />
  );
}

const styles = StyleSheet.create({
  base: {
    padding: 16,
    borderRadius: 8,
    backgroundColor: '#f3f4f6',
  },
  primary: {
    backgroundColor: '#6366f1',
  },
  danger: {
    backgroundColor: '#ef4444',
  },
});
```

## Theme System

### Theme Context

```typescript
import React, { createContext, useContext, useMemo } from 'react';
import { useColorScheme } from 'react-native';

interface Theme {
  colors: {
    primary: string;
    secondary: string;
    background: string;
    surface: string;
    text: string;
    textSecondary: string;
    border: string;
    error: string;
    success: string;
  };
  spacing: {
    xs: number;
    sm: number;
    md: number;
    lg: number;
    xl: number;
  };
  borderRadius: {
    sm: number;
    md: number;
    lg: number;
    full: number;
  };
  typography: {
    h1: { fontSize: number; fontWeight: string; lineHeight: number };
    h2: { fontSize: number; fontWeight: string; lineHeight: number };
    body: { fontSize: number; fontWeight: string; lineHeight: number };
    caption: { fontSize: number; fontWeight: string; lineHeight: number };
  };
}

const lightTheme: Theme = {
  colors: {
    primary: '#6366f1',
    secondary: '#8b5cf6',
    background: '#ffffff',
    surface: '#f9fafb',
    text: '#1f2937',
    textSecondary: '#6b7280',
    border: '#e5e7eb',
    error: '#ef4444',
    success: '#10b981',
  },
  spacing: {
    xs: 4,
    sm: 8,
    md: 16,
    lg: 24,
    xl: 32,
  },
  borderRadius: {
    sm: 4,
    md: 8,
    lg: 16,
    full: 9999,
  },
  typography: {
    h1: { fontSize: 32, fontWeight: '700', lineHeight: 40 },
    h2: { fontSize: 24, fontWeight: '600', lineHeight: 32 },
    body: { fontSize: 16, fontWeight: '400', lineHeight: 24 },
    caption: { fontSize: 12, fontWeight: '400', lineHeight: 16 },
  },
};

const darkTheme: Theme = {
  ...lightTheme,
  colors: {
    primary: '#818cf8',
    secondary: '#a78bfa',
    background: '#111827',
    surface: '#1f2937',
    text: '#f9fafb',
    textSecondary: '#9ca3af',
    border: '#374151',
    error: '#f87171',
    success: '#34d399',
  },
};

const ThemeContext = createContext<Theme>(lightTheme);

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const colorScheme = useColorScheme();
  const theme = useMemo(
    () => (colorScheme === 'dark' ? darkTheme : lightTheme),
    [colorScheme]
  );

  return (
    <ThemeContext.Provider value={theme}>
      {children}
    </ThemeContext.Provider>
  );
}

export function useTheme() {
  return useContext(ThemeContext);
}
```

### Using Theme

```typescript
import { useTheme } from './theme';

function ThemedCard() {
  const theme = useTheme();

  return (
    <View
      style={{
        backgroundColor: theme.colors.surface,
        padding: theme.spacing.md,
        borderRadius: theme.borderRadius.lg,
        borderWidth: 1,
        borderColor: theme.colors.border,
      }}
    >
      <Text
        style={{
          ...theme.typography.h2,
          color: theme.colors.text,
          marginBottom: theme.spacing.sm,
        }}
      >
        Card Title
      </Text>
      <Text
        style={{
          ...theme.typography.body,
          color: theme.colors.textSecondary,
        }}
      >
        Card description text
      </Text>
    </View>
  );
}
```

## Responsive Design

### Screen Dimensions

```typescript
import { Dimensions, useWindowDimensions, PixelRatio } from 'react-native';

// Get dimensions once (may be stale after rotation)
const { width: SCREEN_WIDTH, height: SCREEN_HEIGHT } = Dimensions.get('window');

// Responsive scaling
const guidelineBaseWidth = 375;
const guidelineBaseHeight = 812;

export const scale = (size: number) =>
  (SCREEN_WIDTH / guidelineBaseWidth) * size;

export const verticalScale = (size: number) =>
  (SCREEN_HEIGHT / guidelineBaseHeight) * size;

export const moderateScale = (size: number, factor = 0.5) =>
  size + (scale(size) - size) * factor;

// Hook for dynamic dimensions (handles rotation)
function ResponsiveComponent() {
  const { width, height } = useWindowDimensions();
  const isLandscape = width > height;
  const isTablet = width >= 768;

  return (
    <View style={{ flexDirection: isLandscape ? 'row' : 'column' }}>
      {/* Content */}
    </View>
  );
}
```

### Breakpoint System

```typescript
import { useWindowDimensions } from 'react-native';

type Breakpoint = 'sm' | 'md' | 'lg' | 'xl';

const breakpoints = {
  sm: 0,
  md: 768,
  lg: 1024,
  xl: 1280,
};

export function useBreakpoint(): Breakpoint {
  const { width } = useWindowDimensions();

  if (width >= breakpoints.xl) return 'xl';
  if (width >= breakpoints.lg) return 'lg';
  if (width >= breakpoints.md) return 'md';
  return 'sm';
}

export function useResponsiveValue<T>(values: Partial<Record<Breakpoint, T>>): T | undefined {
  const breakpoint = useBreakpoint();
  const breakpointOrder: Breakpoint[] = ['xl', 'lg', 'md', 'sm'];
  const currentIndex = breakpointOrder.indexOf(breakpoint);

  for (let i = currentIndex; i < breakpointOrder.length; i++) {
    const bp = breakpointOrder[i];
    if (values[bp] !== undefined) {
      return values[bp];
    }
  }

  return undefined;
}

// Usage
function ResponsiveGrid() {
  const columns = useResponsiveValue({ sm: 1, md: 2, lg: 3, xl: 4 }) ?? 1;

  return (
    <View style={{ flexDirection: 'row', flexWrap: 'wrap' }}>
      {items.map((item) => (
        <View key={item.id} style={{ width: `${100 / columns}%` }}>
          <ItemCard item={item} />
        </View>
      ))}
    </View>
  );
}
```

## Layout Components

### Container

```typescript
import { View, ViewStyle, StyleProp } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { useTheme } from './theme';

interface ContainerProps {
  children: React.ReactNode;
  style?: StyleProp<ViewStyle>;
  edges?: ('top' | 'bottom' | 'left' | 'right')[];
}

export function Container({ children, style, edges = ['top', 'bottom'] }: ContainerProps) {
  const insets = useSafeAreaInsets();
  const theme = useTheme();

  return (
    <View
      style={[
        {
          flex: 1,
          backgroundColor: theme.colors.background,
          paddingTop: edges.includes('top') ? insets.top : 0,
          paddingBottom: edges.includes('bottom') ? insets.bottom : 0,
          paddingLeft: edges.includes('left') ? insets.left : 0,
          paddingRight: edges.includes('right') ? insets.right : 0,
        },
        style,
      ]}
    >
      {children}
    </View>
  );
}
```

### Stack Components

```typescript
import { View, ViewStyle, StyleProp } from 'react-native';

interface StackProps {
  children: React.ReactNode;
  spacing?: number;
  style?: StyleProp<ViewStyle>;
}

export function VStack({ children, spacing = 8, style }: StackProps) {
  return (
    <View style={[{ gap: spacing }, style]}>
      {children}
    </View>
  );
}

export function HStack({ children, spacing = 8, style }: StackProps) {
  return (
    <View style={[{ flexDirection: 'row', alignItems: 'center', gap: spacing }, style]}>
      {children}
    </View>
  );
}

// Usage
function Example() {
  return (
    <VStack spacing={16}>
      <HStack spacing={8}>
        <Avatar />
        <VStack spacing={2}>
          <Text style={styles.name}>John Doe</Text>
          <Text style={styles.email}>john@example.com</Text>
        </VStack>
      </HStack>
      <Button title="Edit Profile" />
    </VStack>
  );
}
```

### Spacer

```typescript
import { View } from 'react-native';

interface SpacerProps {
  size?: number;
  flex?: number;
}

export function Spacer({ size, flex }: SpacerProps) {
  if (flex) {
    return <View style={{ flex }} />;
  }
  return <View style={{ height: size, width: size }} />;
}

// Usage
<HStack>
  <Text>Left</Text>
  <Spacer flex={1} />
  <Text>Right</Text>
</HStack>
```

## Shadow Styles

### Cross-Platform Shadows

```typescript
import { Platform, ViewStyle } from "react-native";

export function createShadow(elevation: number, color = "#000000"): ViewStyle {
  if (Platform.OS === "android") {
    return { elevation };
  }

  // iOS shadow mapping based on elevation
  const shadowMap: Record<number, ViewStyle> = {
    1: {
      shadowColor: color,
      shadowOffset: { width: 0, height: 1 },
      shadowOpacity: 0.18,
      shadowRadius: 1,
    },
    2: {
      shadowColor: color,
      shadowOffset: { width: 0, height: 2 },
      shadowOpacity: 0.2,
      shadowRadius: 2,
    },
    4: {
      shadowColor: color,
      shadowOffset: { width: 0, height: 2 },
      shadowOpacity: 0.22,
      shadowRadius: 4,
    },
    8: {
      shadowColor: color,
      shadowOffset: { width: 0, height: 4 },
      shadowOpacity: 0.25,
      shadowRadius: 8,
    },
    16: {
      shadowColor: color,
      shadowOffset: { width: 0, height: 8 },
      shadowOpacity: 0.3,
      shadowRadius: 16,
    },
  };

  return shadowMap[elevation] || shadowMap[4];
}

// Predefined shadow styles
export const shadows = {
  sm: createShadow(2),
  md: createShadow(4),
  lg: createShadow(8),
  xl: createShadow(16),
};

// Usage
const styles = StyleSheet.create({
  card: {
    backgroundColor: "#ffffff",
    borderRadius: 12,
    padding: 16,
    ...shadows.md,
  },
});
```

## Typography System

### Text Components

```typescript
import { Text as RNText, TextStyle, StyleProp, TextProps as RNTextProps } from 'react-native';
import { useTheme } from './theme';

type Variant = 'h1' | 'h2' | 'h3' | 'body' | 'bodySmall' | 'caption' | 'label';
type Color = 'primary' | 'secondary' | 'text' | 'textSecondary' | 'error' | 'success';

interface TextProps extends RNTextProps {
  variant?: Variant;
  color?: Color;
  weight?: 'normal' | 'medium' | 'semibold' | 'bold';
  align?: 'left' | 'center' | 'right';
}

const variantStyles: Record<Variant, TextStyle> = {
  h1: { fontSize: 32, lineHeight: 40, fontWeight: '700' },
  h2: { fontSize: 24, lineHeight: 32, fontWeight: '600' },
  h3: { fontSize: 20, lineHeight: 28, fontWeight: '600' },
  body: { fontSize: 16, lineHeight: 24, fontWeight: '400' },
  bodySmall: { fontSize: 14, lineHeight: 20, fontWeight: '400' },
  caption: { fontSize: 12, lineHeight: 16, fontWeight: '400' },
  label: { fontSize: 14, lineHeight: 20, fontWeight: '500' },
};

const weightStyles: Record<string, TextStyle> = {
  normal: { fontWeight: '400' },
  medium: { fontWeight: '500' },
  semibold: { fontWeight: '600' },
  bold: { fontWeight: '700' },
};

export function Text({
  variant = 'body',
  color = 'text',
  weight,
  align,
  style,
  ...props
}: TextProps) {
  const theme = useTheme();

  return (
    <RNText
      style={[
        variantStyles[variant],
        { color: theme.colors[color] },
        weight && weightStyles[weight],
        align && { textAlign: align },
        style,
      ]}
      {...props}
    />
  );
}

// Usage
<Text variant="h1">Heading</Text>
<Text variant="body" color="textSecondary">Body text</Text>
<Text variant="label" weight="semibold">Label</Text>
```

## Button Styles

### Customizable Button

```typescript
import { Pressable, StyleSheet, ActivityIndicator } from 'react-native';
import Animated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
} from 'react-native-reanimated';
import { useTheme } from './theme';

type Variant = 'filled' | 'outlined' | 'ghost';
type Size = 'sm' | 'md' | 'lg';

interface ButtonProps {
  title: string;
  onPress: () => void;
  variant?: Variant;
  size?: Size;
  disabled?: boolean;
  loading?: boolean;
  leftIcon?: React.ReactNode;
  rightIcon?: React.ReactNode;
}

const AnimatedPressable = Animated.createAnimatedComponent(Pressable);

export function Button({
  title,
  onPress,
  variant = 'filled',
  size = 'md',
  disabled = false,
  loading = false,
  leftIcon,
  rightIcon,
}: ButtonProps) {
  const theme = useTheme();
  const scale = useSharedValue(1);

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [{ scale: scale.value }],
  }));

  const sizeStyles = {
    sm: { paddingVertical: 8, paddingHorizontal: 12, fontSize: 14 },
    md: { paddingVertical: 12, paddingHorizontal: 16, fontSize: 16 },
    lg: { paddingVertical: 16, paddingHorizontal: 24, fontSize: 18 },
  };

  const variantStyles = {
    filled: {
      backgroundColor: theme.colors.primary,
      textColor: '#ffffff',
    },
    outlined: {
      backgroundColor: 'transparent',
      borderWidth: 1,
      borderColor: theme.colors.primary,
      textColor: theme.colors.primary,
    },
    ghost: {
      backgroundColor: 'transparent',
      textColor: theme.colors.primary,
    },
  };

  const currentVariant = variantStyles[variant];
  const currentSize = sizeStyles[size];

  return (
    <AnimatedPressable
      style={[
        styles.base,
        {
          backgroundColor: currentVariant.backgroundColor,
          borderWidth: currentVariant.borderWidth,
          borderColor: currentVariant.borderColor,
          paddingVertical: currentSize.paddingVertical,
          paddingHorizontal: currentSize.paddingHorizontal,
          opacity: disabled ? 0.5 : 1,
        },
        animatedStyle,
      ]}
      onPress={onPress}
      onPressIn={() => { scale.value = withSpring(0.97); }}
      onPressOut={() => { scale.value = withSpring(1); }}
      disabled={disabled || loading}
    >
      {loading ? (
        <ActivityIndicator color={currentVariant.textColor} />
      ) : (
        <>
          {leftIcon}
          <Text
            style={{
              color: currentVariant.textColor,
              fontSize: currentSize.fontSize,
              fontWeight: '600',
              marginHorizontal: leftIcon || rightIcon ? 8 : 0,
            }}
          >
            {title}
          </Text>
          {rightIcon}
        </>
      )}
    </AnimatedPressable>
  );
}

const styles = StyleSheet.create({
  base: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: 8,
  },
});
```

## Form Styles

### Input Component

```typescript
import { useState } from 'react';
import {
  View,
  TextInput,
  StyleSheet,
  TextInputProps,
  Pressable,
} from 'react-native';
import { useTheme } from './theme';

interface InputProps extends TextInputProps {
  label?: string;
  error?: string;
  leftIcon?: React.ReactNode;
  rightIcon?: React.ReactNode;
}

export function Input({
  label,
  error,
  leftIcon,
  rightIcon,
  style,
  ...props
}: InputProps) {
  const theme = useTheme();
  const [isFocused, setIsFocused] = useState(false);

  const borderColor = error
    ? theme.colors.error
    : isFocused
    ? theme.colors.primary
    : theme.colors.border;

  return (
    <View style={styles.container}>
      {label && (
        <Text style={[styles.label, { color: theme.colors.text }]}>
          {label}
        </Text>
      )}
      <View
        style={[
          styles.inputContainer,
          {
            borderColor,
            backgroundColor: theme.colors.surface,
          },
        ]}
      >
        {leftIcon && <View style={styles.icon}>{leftIcon}</View>}
        <TextInput
          style={[
            styles.input,
            { color: theme.colors.text },
            style,
          ]}
          placeholderTextColor={theme.colors.textSecondary}
          onFocus={() => setIsFocused(true)}
          onBlur={() => setIsFocused(false)}
          {...props}
        />
        {rightIcon && <View style={styles.icon}>{rightIcon}</View>}
      </View>
      {error && (
        <Text style={[styles.error, { color: theme.colors.error }]}>
          {error}
        </Text>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    marginBottom: 16,
  },
  label: {
    fontSize: 14,
    fontWeight: '500',
    marginBottom: 6,
  },
  inputContainer: {
    flexDirection: 'row',
    alignItems: 'center',
    borderWidth: 1,
    borderRadius: 8,
    paddingHorizontal: 12,
  },
  input: {
    flex: 1,
    paddingVertical: 12,
    fontSize: 16,
  },
  icon: {
    marginHorizontal: 4,
  },
  error: {
    fontSize: 12,
    marginTop: 4,
  },
});
```

## List Styles

### FlatList with Styling

```typescript
import { FlatList, View, StyleSheet } from 'react-native';

interface Item {
  id: string;
  title: string;
  subtitle: string;
}

function StyledList({ items }: { items: Item[] }) {
  return (
    <FlatList
      data={items}
      keyExtractor={(item) => item.id}
      renderItem={({ item, index }) => (
        <View
          style={[
            styles.item,
            index === 0 && styles.firstItem,
            index === items.length - 1 && styles.lastItem,
          ]}
        >
          <Text style={styles.itemTitle}>{item.title}</Text>
          <Text style={styles.itemSubtitle}>{item.subtitle}</Text>
        </View>
      )}
      ItemSeparatorComponent={() => <View style={styles.separator} />}
      ListHeaderComponent={() => (
        <Text style={styles.header}>List Header</Text>
      )}
      ListEmptyComponent={() => (
        <View style={styles.empty}>
          <Text>No items found</Text>
        </View>
      )}
      contentContainerStyle={styles.listContent}
    />
  );
}

const styles = StyleSheet.create({
  listContent: {
    padding: 16,
  },
  item: {
    backgroundColor: '#ffffff',
    padding: 16,
  },
  firstItem: {
    borderTopLeftRadius: 12,
    borderTopRightRadius: 12,
  },
  lastItem: {
    borderBottomLeftRadius: 12,
    borderBottomRightRadius: 12,
  },
  itemTitle: {
    fontSize: 16,
    fontWeight: '600',
    marginBottom: 4,
  },
  itemSubtitle: {
    fontSize: 14,
    color: '#6b7280',
  },
  separator: {
    height: 1,
    backgroundColor: '#e5e7eb',
  },
  header: {
    fontSize: 20,
    fontWeight: '700',
    marginBottom: 16,
  },
  empty: {
    alignItems: 'center',
    padding: 32,
  },
});
```
