
# Interaction Design

Create engaging, intuitive interactions through motion, feedback, and thoughtful state transitions that enhance usability and delight users.

## When to Use This Skill

- Adding microinteractions to enhance user feedback
- Implementing smooth page and component transitions
- Designing loading states and skeleton screens
- Creating gesture-based interactions
- Building notification and toast systems
- Implementing drag-and-drop interfaces
- Adding scroll-triggered animations
- Designing hover and focus states

## Core Principles

### 1. Purposeful Motion

Motion should communicate, not decorate:

- **Feedback**: Confirm user actions occurred
- **Orientation**: Show where elements come from/go to
- **Focus**: Direct attention to important changes
- **Continuity**: Maintain context during transitions

### 2. Timing Guidelines

| Duration  | Use Case                                  |
| --------- | ----------------------------------------- |
| 100-150ms | Micro-feedback (hovers, clicks)           |
| 200-300ms | Small transitions (toggles, dropdowns)    |
| 300-500ms | Medium transitions (modals, page changes) |
| 500ms+    | Complex choreographed animations          |

### 3. Easing Functions

```css
/* Common easings */
--ease-out: cubic-bezier(0.16, 1, 0.3, 1); /* Decelerate - entering */
--ease-in: cubic-bezier(0.55, 0, 1, 0.45); /* Accelerate - exiting */
--ease-in-out: cubic-bezier(0.65, 0, 0.35, 1); /* Both - moving between */
--spring: cubic-bezier(0.34, 1.56, 0.64, 1); /* Overshoot - playful */
```

## Quick Start: Button Microinteraction

```tsx
import { motion } from "framer-motion";

export function InteractiveButton({ children, onClick }) {
  return (
    <motion.button
      onClick={onClick}
      whileHover={{ scale: 1.02 }}
      whileTap={{ scale: 0.98 }}
      transition={{ type: "spring", stiffness: 400, damping: 17 }}
      className="px-4 py-2 bg-blue-600 text-white rounded-lg"
    >
      {children}
    </motion.button>
  );
}
```

## Interaction Patterns

### 1. Loading States

**Skeleton Screens**: Preserve layout while loading

```tsx
function CardSkeleton() {
  return (
    <div className="animate-pulse">
      <div className="h-48 bg-gray-200 rounded-lg" />
      <div className="mt-4 h-4 bg-gray-200 rounded w-3/4" />
      <div className="mt-2 h-4 bg-gray-200 rounded w-1/2" />
    </div>
  );
}
```

**Progress Indicators**: Show determinate progress

```tsx
function ProgressBar({ progress }: { progress: number }) {
  return (
    <div className="h-2 bg-gray-200 rounded-full overflow-hidden">
      <motion.div
        className="h-full bg-blue-600"
        initial={{ width: 0 }}
        animate={{ width: `${progress}%` }}
        transition={{ ease: "easeOut" }}
      />
    </div>
  );
}
```

### 2. State Transitions

**Toggle with smooth transition**:

```tsx
function Toggle({ checked, onChange }) {
  return (
    <button
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      className={`
        relative w-12 h-6 rounded-full transition-colors duration-200
        ${checked ? "bg-blue-600" : "bg-gray-300"}
      `}
    >
      <motion.span
        className="absolute top-1 left-1 w-4 h-4 bg-white rounded-full shadow"
        animate={{ x: checked ? 24 : 0 }}
        transition={{ type: "spring", stiffness: 500, damping: 30 }}
      />
    </button>
  );
}
```

### 3. Page Transitions

**Framer Motion layout animations**:

```tsx
import { AnimatePresence, motion } from "framer-motion";

function PageTransition({ children, key }) {
  return (
    <AnimatePresence mode="wait">
      <motion.div
        key={key}
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        exit={{ opacity: 0, y: -20 }}
        transition={{ duration: 0.3 }}
      >
        {children}
      </motion.div>
    </AnimatePresence>
  );
}
```

### 4. Feedback Patterns

**Ripple effect on click**:

```tsx
function RippleButton({ children, onClick }) {
  const [ripples, setRipples] = useState([]);

  const handleClick = (e) => {
    const rect = e.currentTarget.getBoundingClientRect();
    const ripple = {
      x: e.clientX - rect.left,
      y: e.clientY - rect.top,
      id: Date.now(),
    };
    setRipples((prev) => [...prev, ripple]);
    setTimeout(() => {
      setRipples((prev) => prev.filter((r) => r.id !== ripple.id));
    }, 600);
    onClick?.(e);
  };

  return (
    <button onClick={handleClick} className="relative overflow-hidden">
      {children}
      {ripples.map((ripple) => (
        <span
          key={ripple.id}
          className="absolute bg-white/30 rounded-full animate-ripple"
          style={{ left: ripple.x, top: ripple.y }}
        />
      ))}
    </button>
  );
}
```

### 5. Gesture Interactions

**Swipe to dismiss**:

```tsx
function SwipeCard({ children, onDismiss }) {
  return (
    <motion.div
      drag="x"
      dragConstraints={{ left: 0, right: 0 }}
      onDragEnd={(_, info) => {
        if (Math.abs(info.offset.x) > 100) {
          onDismiss();
        }
      }}
      className="cursor-grab active:cursor-grabbing"
    >
      {children}
    </motion.div>
  );
}
```

## CSS Animation Patterns

### Keyframe Animations

```css
@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.5;
  }
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.animate-fadeIn {
  animation: fadeIn 0.3s ease-out;
}
.animate-pulse {
  animation: pulse 2s ease-in-out infinite;
}
.animate-spin {
  animation: spin 1s linear infinite;
}
```

### CSS Transitions

```css
.card {
  transition:
    transform 0.2s ease-out,
    box-shadow 0.2s ease-out;
}

.card:hover {
  transform: translateY(-4px);
  box-shadow: 0 12px 24px rgba(0, 0, 0, 0.1);
}
```

## Accessibility Considerations

```css
/* Respect user motion preferences */
@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
  }
}
```

```tsx
function AnimatedComponent() {
  const prefersReducedMotion = window.matchMedia(
    "(prefers-reduced-motion: reduce)",
  ).matches;

  return (
    <motion.div
      animate={{ opacity: 1 }}
      transition={{ duration: prefersReducedMotion ? 0 : 0.3 }}
    />
  );
}
```

## Best Practices

1. **Performance First**: Use `transform` and `opacity` for smooth 60fps
2. **Reduce Motion Support**: Always respect `prefers-reduced-motion`
3. **Consistent Timing**: Use a timing scale across the app
4. **Natural Physics**: Prefer spring animations over linear
5. **Interruptible**: Allow users to cancel long animations
6. **Progressive Enhancement**: Work without JS animations
7. **Test on Devices**: Performance varies significantly

## Common Issues

- **Janky Animations**: Avoid animating `width`, `height`, `top`, `left`
- **Over-animation**: Too much motion causes fatigue
- **Blocking Interactions**: Never prevent user input during animations
- **Memory Leaks**: Clean up animation listeners on unmount
- **Flash of Content**: Use `will-change` sparingly for optimization


---

## อ้างอิง: animation-libraries

# Animation Libraries Reference

## Framer Motion

The most popular React animation library with declarative API.

### Basic Animations

```tsx
import { motion, AnimatePresence } from "framer-motion";

// Simple animation
function FadeIn({ children }) {
  return (
    <motion.div
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      transition={{ duration: 0.3 }}
    >
      {children}
    </motion.div>
  );
}

// Gesture animations
function InteractiveCard() {
  return (
    <motion.div
      whileHover={{ scale: 1.02, y: -4 }}
      whileTap={{ scale: 0.98 }}
      transition={{ type: "spring", stiffness: 400, damping: 17 }}
      className="p-6 bg-white rounded-lg shadow"
    >
      Hover or tap me
    </motion.div>
  );
}

// Keyframes animation
function PulseButton() {
  return (
    <motion.button
      animate={{
        scale: [1, 1.05, 1],
        boxShadow: [
          "0 0 0 0 rgba(59, 130, 246, 0.5)",
          "0 0 0 10px rgba(59, 130, 246, 0)",
          "0 0 0 0 rgba(59, 130, 246, 0)",
        ],
      }}
      transition={{ duration: 2, repeat: Infinity }}
      className="px-4 py-2 bg-blue-600 text-white rounded"
    >
      Click me
    </motion.button>
  );
}
```

### Layout Animations

```tsx
import { motion, LayoutGroup } from "framer-motion";

// Shared layout animation
function TabIndicator({ activeTab, tabs }) {
  return (
    <div className="flex border-b">
      {tabs.map((tab) => (
        <button
          key={tab.id}
          onClick={() => setActiveTab(tab.id)}
          className="relative px-4 py-2"
        >
          {tab.label}
          {activeTab === tab.id && (
            <motion.div
              layoutId="activeTab"
              className="absolute bottom-0 left-0 right-0 h-0.5 bg-blue-600"
              transition={{ type: "spring", stiffness: 500, damping: 30 }}
            />
          )}
        </button>
      ))}
    </div>
  );
}

// Auto-layout reordering
function ReorderableList({ items, setItems }) {
  return (
    <Reorder.Group axis="y" values={items} onReorder={setItems}>
      {items.map((item) => (
        <Reorder.Item
          key={item.id}
          value={item}
          className="bg-white p-4 rounded-lg shadow mb-2 cursor-grab active:cursor-grabbing"
        >
          {item.title}
        </Reorder.Item>
      ))}
    </Reorder.Group>
  );
}
```

### Orchestration

```tsx
// Staggered children
const containerVariants = {
  hidden: { opacity: 0 },
  visible: {
    opacity: 1,
    transition: {
      staggerChildren: 0.1,
      delayChildren: 0.2,
    },
  },
};

const itemVariants = {
  hidden: { opacity: 0, y: 20 },
  visible: {
    opacity: 1,
    y: 0,
    transition: { duration: 0.3 },
  },
};

function StaggeredList({ items }) {
  return (
    <motion.ul variants={containerVariants} initial="hidden" animate="visible">
      {items.map((item) => (
        <motion.li key={item.id} variants={itemVariants}>
          {item.content}
        </motion.li>
      ))}
    </motion.ul>
  );
}
```

### Page Transitions

```tsx
import { AnimatePresence, motion } from "framer-motion";
import { useRouter } from "next/router";

const pageVariants = {
  initial: { opacity: 0, x: -20 },
  enter: { opacity: 1, x: 0 },
  exit: { opacity: 0, x: 20 },
};

function PageTransition({ children }) {
  const router = useRouter();

  return (
    <AnimatePresence mode="wait" initial={false}>
      <motion.div
        key={router.pathname}
        variants={pageVariants}
        initial="initial"
        animate="enter"
        exit="exit"
        transition={{ duration: 0.3 }}
      >
        {children}
      </motion.div>
    </AnimatePresence>
  );
}
```

## GSAP (GreenSock)

Industry-standard animation library for complex, performant animations.

### Basic Timeline

```tsx
import { useRef, useLayoutEffect } from "react";
import gsap from "gsap";

function AnimatedHero() {
  const containerRef = useRef<HTMLDivElement>(null);
  const titleRef = useRef<HTMLHeadingElement>(null);
  const subtitleRef = useRef<HTMLParagraphElement>(null);

  useLayoutEffect(() => {
    const ctx = gsap.context(() => {
      const tl = gsap.timeline({ defaults: { ease: "power3.out" } });

      tl.from(titleRef.current, {
        y: 50,
        opacity: 0,
        duration: 0.8,
      })
        .from(
          subtitleRef.current,
          {
            y: 30,
            opacity: 0,
            duration: 0.6,
          },
          "-=0.4", // Start 0.4s before previous ends
        )
        .from(".cta-button", {
          scale: 0.8,
          opacity: 0,
          duration: 0.4,
        });
    }, containerRef);

    return () => ctx.revert(); // Cleanup
  }, []);

  return (
    <div ref={containerRef}>
      <h1 ref={titleRef}>Welcome</h1>
      <p ref={subtitleRef}>Discover amazing things</p>
      <button className="cta-button">Get Started</button>
    </div>
  );
}
```

### ScrollTrigger

```tsx
import { useLayoutEffect, useRef } from "react";
import gsap from "gsap";
import { ScrollTrigger } from "gsap/ScrollTrigger";

gsap.registerPlugin(ScrollTrigger);

function ParallaxSection() {
  const sectionRef = useRef<HTMLDivElement>(null);
  const imageRef = useRef<HTMLImageElement>(null);

  useLayoutEffect(() => {
    const ctx = gsap.context(() => {
      // Parallax image
      gsap.to(imageRef.current, {
        yPercent: -20,
        ease: "none",
        scrollTrigger: {
          trigger: sectionRef.current,
          start: "top bottom",
          end: "bottom top",
          scrub: true,
        },
      });

      // Fade in content
      gsap.from(".content-block", {
        opacity: 0,
        y: 50,
        stagger: 0.2,
        scrollTrigger: {
          trigger: sectionRef.current,
          start: "top 80%",
          end: "top 20%",
          scrub: 1,
        },
      });
    }, sectionRef);

    return () => ctx.revert();
  }, []);

  return (
    <section ref={sectionRef} className="relative overflow-hidden">
      <img ref={imageRef} src="/hero.jpg" alt="" className="absolute inset-0" />
      <div className="relative z-10">
        <div className="content-block">Block 1</div>
        <div className="content-block">Block 2</div>
      </div>
    </section>
  );
}
```

### Text Animation

```tsx
import { useLayoutEffect, useRef } from "react";
import gsap from "gsap";
import { SplitText } from "gsap/SplitText";

gsap.registerPlugin(SplitText);

function AnimatedHeadline({ text }) {
  const textRef = useRef<HTMLHeadingElement>(null);

  useLayoutEffect(() => {
    const split = new SplitText(textRef.current, {
      type: "chars,words",
      charsClass: "char",
    });

    gsap.from(split.chars, {
      opacity: 0,
      y: 50,
      rotateX: -90,
      stagger: 0.02,
      duration: 0.8,
      ease: "back.out(1.7)",
    });

    return () => split.revert();
  }, [text]);

  return <h1 ref={textRef}>{text}</h1>;
}
```

## CSS Spring Physics

```tsx
// spring.ts - Custom spring physics
interface SpringConfig {
  stiffness: number; // Higher = snappier
  damping: number; // Higher = less bouncy
  mass: number;
}

const presets: Record<string, SpringConfig> = {
  default: { stiffness: 170, damping: 26, mass: 1 },
  gentle: { stiffness: 120, damping: 14, mass: 1 },
  wobbly: { stiffness: 180, damping: 12, mass: 1 },
  stiff: { stiffness: 210, damping: 20, mass: 1 },
  slow: { stiffness: 280, damping: 60, mass: 1 },
  molasses: { stiffness: 280, damping: 120, mass: 1 },
};

function springToCss(config: SpringConfig): string {
  // Convert spring parameters to CSS timing function approximation
  const { stiffness, damping } = config;
  const duration = Math.sqrt(stiffness) / damping;
  const bounce = 1 - damping / (2 * Math.sqrt(stiffness));

  // Map to cubic-bezier (approximation)
  if (bounce <= 0) {
    return `cubic-bezier(0.25, 0.1, 0.25, 1)`;
  }
  return `cubic-bezier(0.34, 1.56, 0.64, 1)`;
}
```

## Web Animations API

Native browser animation API for simple animations.

```tsx
function useWebAnimation(
  ref: RefObject<HTMLElement>,
  keyframes: Keyframe[],
  options: KeyframeAnimationOptions,
) {
  useEffect(() => {
    if (!ref.current) return;

    const animation = ref.current.animate(keyframes, options);

    return () => animation.cancel();
  }, [ref, keyframes, options]);
}

// Usage
function SlideIn({ children }) {
  const elementRef = useRef<HTMLDivElement>(null);

  useWebAnimation(
    elementRef,
    [
      { transform: "translateX(-100%)", opacity: 0 },
      { transform: "translateX(0)", opacity: 1 },
    ],
    {
      duration: 300,
      easing: "cubic-bezier(0.16, 1, 0.3, 1)",
      fill: "forwards",
    },
  );

  return <div ref={elementRef}>{children}</div>;
}
```

## View Transitions API

Native browser API for page transitions.

```tsx
// Check support
const supportsViewTransitions = "startViewTransition" in document;

// Simple page transition
async function navigateTo(url: string) {
  if (!document.startViewTransition) {
    window.location.href = url;
    return;
  }

  document.startViewTransition(async () => {
    await fetch(url);
    // Update DOM
  });
}

// Named elements for morphing
function ProductCard({ product }) {
  return (
    <Link href={`/product/${product.id}`}>
      <img
        src={product.image}
        style={{ viewTransitionName: `product-${product.id}` }}
      />
    </Link>
  );
}

// CSS for view transitions
/*
::view-transition-old(root) {
  animation: fade-out 0.25s ease-out;
}

::view-transition-new(root) {
  animation: fade-in 0.25s ease-in;
}

::view-transition-group(product-*) {
  animation-duration: 0.3s;
}
*/
```

## Performance Tips

### GPU Acceleration

```css
/* Properties that trigger GPU acceleration */
.animated-element {
  transform: translateZ(0); /* Force GPU layer */
  will-change: transform, opacity; /* Hint to browser */
}

/* Only animate transform and opacity for 60fps */
.smooth {
  transition:
    transform 0.3s ease,
    opacity 0.3s ease;
}

/* Avoid animating these (cause reflow) */
.avoid {
  /* Don't animate: width, height, top, left, margin, padding */
}
```

### Reduced Motion

```tsx
function useReducedMotion() {
  const [prefersReduced, setPrefersReduced] = useState(false);

  useEffect(() => {
    const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
    setPrefersReduced(mq.matches);

    const handler = (e: MediaQueryListEvent) => setPrefersReduced(e.matches);
    mq.addEventListener("change", handler);
    return () => mq.removeEventListener("change", handler);
  }, []);

  return prefersReduced;
}

// Usage
function AnimatedComponent() {
  const prefersReduced = useReducedMotion();

  return (
    <motion.div
      initial={{ opacity: 0, y: prefersReduced ? 0 : 20 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: prefersReduced ? 0 : 0.3 }}
    >
      Content
    </motion.div>
  );
}
```


---

## อ้างอิง: microinteraction-patterns

# Microinteraction Patterns Reference

## Button States

### Loading Button

```tsx
import { motion, AnimatePresence } from "framer-motion";

interface LoadingButtonProps {
  isLoading: boolean;
  children: React.ReactNode;
  onClick: () => void;
}

function LoadingButton({ isLoading, children, onClick }: LoadingButtonProps) {
  return (
    <button
      onClick={onClick}
      disabled={isLoading}
      className="relative px-4 py-2 bg-blue-600 text-white rounded-lg overflow-hidden"
    >
      <AnimatePresence mode="wait">
        {isLoading ? (
          <motion.span
            key="loading"
            initial={{ opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -10 }}
            className="flex items-center gap-2"
          >
            <Spinner className="w-4 h-4" />
            Processing...
          </motion.span>
        ) : (
          <motion.span
            key="idle"
            initial={{ opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -10 }}
          >
            {children}
          </motion.span>
        )}
      </AnimatePresence>
    </button>
  );
}

// Spinner component
function Spinner({ className }: { className?: string }) {
  return (
    <svg className={`animate-spin ${className}`} viewBox="0 0 24 24">
      <circle
        cx="12"
        cy="12"
        r="10"
        stroke="currentColor"
        strokeWidth="4"
        fill="none"
        strokeDasharray="62.83"
        strokeDashoffset="15"
        strokeLinecap="round"
      />
    </svg>
  );
}
```

### Success/Error State

```tsx
function SubmitButton({ onSubmit }: { onSubmit: () => Promise<void> }) {
  const [state, setState] = useState<"idle" | "loading" | "success" | "error">(
    "idle",
  );

  const handleClick = async () => {
    setState("loading");
    try {
      await onSubmit();
      setState("success");
      setTimeout(() => setState("idle"), 2000);
    } catch {
      setState("error");
      setTimeout(() => setState("idle"), 2000);
    }
  };

  const icons = {
    idle: null,
    loading: <Spinner className="w-5 h-5" />,
    success: <CheckIcon className="w-5 h-5" />,
    error: <XIcon className="w-5 h-5" />,
  };

  const colors = {
    idle: "bg-blue-600 hover:bg-blue-700",
    loading: "bg-blue-600",
    success: "bg-green-600",
    error: "bg-red-600",
  };

  return (
    <motion.button
      onClick={handleClick}
      disabled={state === "loading"}
      className={`flex items-center gap-2 px-4 py-2 text-white rounded-lg transition-colors ${colors[state]}`}
      animate={{
        scale: state === "success" || state === "error" ? [1, 1.05, 1] : 1,
      }}
    >
      <AnimatePresence mode="wait">
        {icons[state] && (
          <motion.span
            key={state}
            initial={{ scale: 0, rotate: -180 }}
            animate={{ scale: 1, rotate: 0 }}
            exit={{ scale: 0, rotate: 180 }}
          >
            {icons[state]}
          </motion.span>
        )}
      </AnimatePresence>
      {state === "idle" && "Submit"}
      {state === "loading" && "Submitting..."}
      {state === "success" && "Done!"}
      {state === "error" && "Failed"}
    </motion.button>
  );
}
```

## Form Interactions

### Floating Label Input

```tsx
import { useState, useId } from "react";

function FloatingInput({
  label,
  type = "text",
}: {
  label: string;
  type?: string;
}) {
  const [value, setValue] = useState("");
  const [isFocused, setIsFocused] = useState(false);
  const id = useId();

  const isFloating = isFocused || value.length > 0;

  return (
    <div className="relative">
      <input
        id={id}
        type={type}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onFocus={() => setIsFocused(true)}
        onBlur={() => setIsFocused(false)}
        className="peer w-full px-4 py-3 border rounded-lg outline-none transition-colors
          focus:border-blue-600 focus:ring-2 focus:ring-blue-100"
      />
      <label
        htmlFor={id}
        className={`absolute left-4 transition-all duration-200 pointer-events-none
          ${
            isFloating
              ? "top-0 -translate-y-1/2 text-xs bg-white px-1 text-blue-600"
              : "top-1/2 -translate-y-1/2 text-gray-500"
          }`}
      >
        {label}
      </label>
    </div>
  );
}
```

### Shake on Error

```tsx
import { motion, useAnimation } from "framer-motion";

function ShakeInput({ error, ...props }: InputProps & { error?: string }) {
  const controls = useAnimation();

  useEffect(() => {
    if (error) {
      controls.start({
        x: [0, -10, 10, -10, 10, 0],
        transition: { duration: 0.4 },
      });
    }
  }, [error, controls]);

  return (
    <motion.div animate={controls}>
      <input
        {...props}
        className={`w-full px-4 py-2 border rounded-lg ${
          error ? "border-red-500" : "border-gray-300"
        }`}
      />
      {error && (
        <motion.p
          initial={{ opacity: 0, y: -10 }}
          animate={{ opacity: 1, y: 0 }}
          className="mt-1 text-sm text-red-500"
        >
          {error}
        </motion.p>
      )}
    </motion.div>
  );
}
```

### Character Count

```tsx
function TextareaWithCount({ maxLength = 280 }: { maxLength?: number }) {
  const [value, setValue] = useState("");
  const remaining = maxLength - value.length;
  const isNearLimit = remaining <= 20;
  const isOverLimit = remaining < 0;

  return (
    <div className="relative">
      <textarea
        value={value}
        onChange={(e) => setValue(e.target.value)}
        className="w-full px-4 py-3 border rounded-lg resize-none"
        rows={4}
      />
      <motion.span
        className={`absolute bottom-2 right-2 text-sm ${
          isOverLimit
            ? "text-red-500"
            : isNearLimit
              ? "text-yellow-500"
              : "text-gray-400"
        }`}
        animate={{ scale: isNearLimit ? [1, 1.1, 1] : 1 }}
        transition={{ duration: 0.2 }}
      >
        {remaining}
      </motion.span>
    </div>
  );
}
```

## Feedback Patterns

### Toast Notifications

```tsx
import { motion, AnimatePresence } from "framer-motion";
import { createContext, useContext, useState, useCallback } from "react";

interface Toast {
  id: string;
  message: string;
  type: "success" | "error" | "info";
}

const ToastContext = createContext<{
  addToast: (message: string, type: Toast["type"]) => void;
} | null>(null);

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const addToast = useCallback((message: string, type: Toast["type"]) => {
    const id = Date.now().toString();
    setToasts((prev) => [...prev, { id, message, type }]);
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, 3000);
  }, []);

  return (
    <ToastContext.Provider value={{ addToast }}>
      {children}
      <div className="fixed bottom-4 right-4 space-y-2 z-50">
        <AnimatePresence>
          {toasts.map((toast) => (
            <motion.div
              key={toast.id}
              initial={{ opacity: 0, y: 20, scale: 0.95 }}
              animate={{ opacity: 1, y: 0, scale: 1 }}
              exit={{ opacity: 0, x: 100, scale: 0.95 }}
              className={`px-4 py-3 rounded-lg shadow-lg ${
                toast.type === "success"
                  ? "bg-green-600"
                  : toast.type === "error"
                    ? "bg-red-600"
                    : "bg-blue-600"
              } text-white`}
            >
              {toast.message}
            </motion.div>
          ))}
        </AnimatePresence>
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) throw new Error("useToast must be within ToastProvider");
  return context;
}
```

### Confirmation Dialog

```tsx
function ConfirmButton({
  onConfirm,
  confirmText = "Click again to confirm",
  children,
}: {
  onConfirm: () => void;
  confirmText?: string;
  children: React.ReactNode;
}) {
  const [isPending, setIsPending] = useState(false);

  useEffect(() => {
    if (isPending) {
      const timer = setTimeout(() => setIsPending(false), 3000);
      return () => clearTimeout(timer);
    }
  }, [isPending]);

  const handleClick = () => {
    if (isPending) {
      onConfirm();
      setIsPending(false);
    } else {
      setIsPending(true);
    }
  };

  return (
    <motion.button
      onClick={handleClick}
      className={`px-4 py-2 rounded-lg transition-colors ${
        isPending ? "bg-red-600 text-white" : "bg-gray-200 text-gray-800"
      }`}
      animate={{ scale: isPending ? [1, 1.02, 1] : 1 }}
    >
      <AnimatePresence mode="wait">
        <motion.span
          key={isPending ? "confirm" : "idle"}
          initial={{ opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -10 }}
        >
          {isPending ? confirmText : children}
        </motion.span>
      </AnimatePresence>
    </motion.button>
  );
}
```

## Navigation Patterns

### Active Link Indicator

```tsx
import { motion } from "framer-motion";
import { usePathname } from "next/navigation";

function Navigation({ items }: { items: { href: string; label: string }[] }) {
  const pathname = usePathname();

  return (
    <nav className="flex gap-1 p-1 bg-gray-100 rounded-lg">
      {items.map((item) => {
        const isActive = pathname === item.href;
        return (
          <Link
            key={item.href}
            href={item.href}
            className={`relative px-4 py-2 text-sm font-medium ${
              isActive ? "text-white" : "text-gray-600 hover:text-gray-900"
            }`}
          >
            {isActive && (
              <motion.div
                layoutId="activeNav"
                className="absolute inset-0 bg-blue-600 rounded-md"
                transition={{ type: "spring", stiffness: 500, damping: 30 }}
              />
            )}
            <span className="relative z-10">{item.label}</span>
          </Link>
        );
      })}
    </nav>
  );
}
```

### Hamburger Menu Icon

```tsx
function MenuIcon({ isOpen }: { isOpen: boolean }) {
  return (
    <button className="relative w-6 h-6" aria-label="Toggle menu">
      <motion.span
        className="absolute left-0 h-0.5 w-6 bg-current"
        animate={{
          top: isOpen ? "50%" : "25%",
          rotate: isOpen ? 45 : 0,
          translateY: isOpen ? "-50%" : 0,
        }}
        transition={{ duration: 0.2 }}
      />
      <motion.span
        className="absolute left-0 top-1/2 h-0.5 w-6 bg-current -translate-y-1/2"
        animate={{ opacity: isOpen ? 0 : 1, scaleX: isOpen ? 0 : 1 }}
        transition={{ duration: 0.2 }}
      />
      <motion.span
        className="absolute left-0 h-0.5 w-6 bg-current"
        animate={{
          bottom: isOpen ? "50%" : "25%",
          rotate: isOpen ? -45 : 0,
          translateY: isOpen ? "50%" : 0,
        }}
        transition={{ duration: 0.2 }}
      />
    </button>
  );
}
```

## Data Interactions

### Optimistic Updates

```tsx
function LikeButton({ postId, initialLiked, initialCount }) {
  const [liked, setLiked] = useState(initialLiked);
  const [count, setCount] = useState(initialCount);

  const handleLike = async () => {
    // Optimistic update
    const newLiked = !liked;
    setLiked(newLiked);
    setCount((c) => c + (newLiked ? 1 : -1));

    try {
      await api.toggleLike(postId);
    } catch {
      // Rollback on error
      setLiked(!newLiked);
      setCount((c) => c + (newLiked ? -1 : 1));
    }
  };

  return (
    <motion.button
      onClick={handleLike}
      whileTap={{ scale: 0.9 }}
      className={`flex items-center gap-2 ${liked ? "text-red-500" : "text-gray-500"}`}
    >
      <motion.span
        animate={{ scale: liked ? [1, 1.3, 1] : 1 }}
        transition={{ duration: 0.3 }}
      >
        {liked ? <HeartFilledIcon /> : <HeartIcon />}
      </motion.span>
      <AnimatePresence mode="wait">
        <motion.span
          key={count}
          initial={{ opacity: 0, y: -10 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: 10 }}
        >
          {count}
        </motion.span>
      </AnimatePresence>
    </motion.button>
  );
}
```

### Pull to Refresh

```tsx
import { motion, useMotionValue, useTransform } from "framer-motion";

function PullToRefresh({ onRefresh, children }) {
  const y = useMotionValue(0);
  const [isRefreshing, setIsRefreshing] = useState(false);

  const opacity = useTransform(y, [0, 60], [0, 1]);
  const rotate = useTransform(y, [0, 60], [0, 180]);

  const handleDragEnd = async (_, info) => {
    if (info.offset.y > 60 && !isRefreshing) {
      setIsRefreshing(true);
      await onRefresh();
      setIsRefreshing(false);
    }
  };

  return (
    <div className="overflow-hidden">
      <motion.div style={{ opacity }} className="flex justify-center py-4">
        <motion.div style={{ rotate }}>
          {isRefreshing ? <Spinner /> : <ArrowDownIcon />}
        </motion.div>
      </motion.div>
      <motion.div
        drag="y"
        dragConstraints={{ top: 0, bottom: 0 }}
        dragElastic={{ top: 0.5, bottom: 0 }}
        style={{ y }}
        onDragEnd={handleDragEnd}
      >
        {children}
      </motion.div>
    </div>
  );
}
```


---

## อ้างอิง: scroll-animations

# Scroll Animations Reference

## Intersection Observer Hook

```tsx
import { useEffect, useRef, useState, type RefObject } from "react";

interface UseInViewOptions {
  threshold?: number | number[];
  rootMargin?: string;
  triggerOnce?: boolean;
}

function useInView<T extends HTMLElement>({
  threshold = 0,
  rootMargin = "0px",
  triggerOnce = false,
}: UseInViewOptions = {}): [RefObject<T>, boolean] {
  const ref = useRef<T>(null);
  const [isInView, setIsInView] = useState(false);

  useEffect(() => {
    const element = ref.current;
    if (!element) return;

    const observer = new IntersectionObserver(
      ([entry]) => {
        const inView = entry.isIntersecting;
        setIsInView(inView);
        if (inView && triggerOnce) {
          observer.unobserve(element);
        }
      },
      { threshold, rootMargin },
    );

    observer.observe(element);
    return () => observer.disconnect();
  }, [threshold, rootMargin, triggerOnce]);

  return [ref, isInView];
}

// Usage
function FadeInSection({ children }) {
  const [ref, isInView] = useInView({ threshold: 0.2, triggerOnce: true });

  return (
    <div
      ref={ref}
      className={`transition-all duration-700 ${
        isInView ? "opacity-100 translate-y-0" : "opacity-0 translate-y-8"
      }`}
    >
      {children}
    </div>
  );
}
```

## Scroll Progress Indicator

```tsx
import { motion, useScroll, useSpring } from "framer-motion";

function ScrollProgress() {
  const { scrollYProgress } = useScroll();
  const scaleX = useSpring(scrollYProgress, {
    stiffness: 100,
    damping: 30,
    restDelta: 0.001,
  });

  return (
    <motion.div
      className="fixed top-0 left-0 right-0 h-1 bg-blue-600 origin-left z-50"
      style={{ scaleX }}
    />
  );
}
```

## Parallax Scrolling

### Simple CSS Parallax

```css
.parallax-container {
  height: 100vh;
  overflow-x: hidden;
  overflow-y: auto;
  perspective: 10px;
}

.parallax-layer-back {
  transform: translateZ(-10px) scale(2);
}

.parallax-layer-base {
  transform: translateZ(0);
}
```

### Framer Motion Parallax

```tsx
import { motion, useScroll, useTransform } from "framer-motion";

function ParallaxHero() {
  const ref = useRef(null);
  const { scrollYProgress } = useScroll({
    target: ref,
    offset: ["start start", "end start"],
  });

  const y = useTransform(scrollYProgress, [0, 1], ["0%", "50%"]);
  const opacity = useTransform(scrollYProgress, [0, 0.5], [1, 0]);
  const scale = useTransform(scrollYProgress, [0, 1], [1, 1.2]);

  return (
    <section ref={ref} className="relative h-screen overflow-hidden">
      {/* Background image with parallax */}
      <motion.div style={{ y, scale }} className="absolute inset-0">
        <img src="/hero-bg.jpg" alt="" className="w-full h-full object-cover" />
      </motion.div>

      {/* Content fades out on scroll */}
      <motion.div
        style={{ opacity }}
        className="relative z-10 flex items-center justify-center h-full"
      >
        <h1 className="text-6xl font-bold text-white">Welcome</h1>
      </motion.div>
    </section>
  );
}
```

## Scroll-Linked Animations

### Progress-Based Animation

```tsx
function ScrollAnimation() {
  const containerRef = useRef(null);
  const { scrollYProgress } = useScroll({
    target: containerRef,
    offset: ["start end", "end start"],
  });

  // Different transformations based on scroll progress
  const x = useTransform(scrollYProgress, [0, 1], [-200, 200]);
  const rotate = useTransform(scrollYProgress, [0, 1], [0, 360]);
  const backgroundColor = useTransform(
    scrollYProgress,
    [0, 0.5, 1],
    ["#3b82f6", "#8b5cf6", "#ec4899"],
  );

  return (
    <div ref={containerRef} className="h-[200vh] py-20">
      <div className="sticky top-1/2 -translate-y-1/2 flex justify-center">
        <motion.div
          style={{ x, rotate, backgroundColor }}
          className="w-32 h-32 rounded-2xl"
        />
      </div>
    </div>
  );
}
```

### Horizontal Scroll Section

```tsx
function HorizontalScroll({ items }) {
  const containerRef = useRef(null);
  const { scrollYProgress } = useScroll({
    target: containerRef,
    offset: ["start start", "end end"],
  });

  const x = useTransform(
    scrollYProgress,
    [0, 1],
    ["0%", `-${(items.length - 1) * 100}%`],
  );

  return (
    <section ref={containerRef} className="relative h-[300vh]">
      <div className="sticky top-0 h-screen overflow-hidden">
        <motion.div style={{ x }} className="flex h-full">
          {items.map((item, i) => (
            <div
              key={i}
              className="flex-shrink-0 w-screen h-full flex items-center justify-center"
            >
              {item}
            </div>
          ))}
        </motion.div>
      </div>
    </section>
  );
}
```

## Reveal Animations

### Staggered List Reveal

```tsx
function StaggeredList({ items }) {
  const [ref, isInView] = useInView({ threshold: 0.1, triggerOnce: true });

  return (
    <ul ref={ref} className="space-y-4">
      {items.map((item, i) => (
        <motion.li
          key={item.id}
          initial={{ opacity: 0, x: -20 }}
          animate={isInView ? { opacity: 1, x: 0 } : {}}
          transition={{ delay: i * 0.1, duration: 0.5 }}
          className="p-4 bg-white rounded-lg shadow"
        >
          {item.content}
        </motion.li>
      ))}
    </ul>
  );
}
```

### Text Reveal

```tsx
function TextReveal({ text }) {
  const [ref, isInView] = useInView({ threshold: 0.5, triggerOnce: true });
  const words = text.split(" ");

  return (
    <p ref={ref} className="text-4xl font-bold">
      {words.map((word, i) => (
        <motion.span
          key={i}
          initial={{ opacity: 0, y: 20 }}
          animate={isInView ? { opacity: 1, y: 0 } : {}}
          transition={{ delay: i * 0.05, duration: 0.3 }}
          className="inline-block mr-2"
        >
          {word}
        </motion.span>
      ))}
    </p>
  );
}
```

### Clip Path Reveal

```tsx
function ClipReveal({ children }) {
  const [ref, isInView] = useInView({ threshold: 0.3, triggerOnce: true });

  return (
    <motion.div
      ref={ref}
      initial={{ clipPath: "inset(0 100% 0 0)" }}
      animate={isInView ? { clipPath: "inset(0 0% 0 0)" } : {}}
      transition={{ duration: 0.8, ease: [0.16, 1, 0.3, 1] }}
    >
      {children}
    </motion.div>
  );
}
```

## Sticky Scroll Sections

```tsx
function StickySection({ title, content, image }) {
  const ref = useRef(null);
  const { scrollYProgress } = useScroll({
    target: ref,
    offset: ["start start", "end start"],
  });

  const opacity = useTransform(scrollYProgress, [0, 0.5, 1], [1, 1, 0]);
  const scale = useTransform(scrollYProgress, [0, 0.5, 1], [1, 1, 0.8]);

  return (
    <section ref={ref} className="relative h-[200vh]">
      <motion.div
        style={{ opacity, scale }}
        className="sticky top-0 h-screen flex items-center"
      >
        <div className="grid grid-cols-2 gap-16 container mx-auto">
          <div>
            <h2 className="text-4xl font-bold">{title}</h2>
            <p className="mt-4 text-lg text-gray-600">{content}</p>
          </div>
          <div>
            <img src={image} alt="" className="rounded-2xl shadow-xl" />
          </div>
        </div>
      </motion.div>
    </section>
  );
}
```

## Smooth Scroll

```tsx
// Using CSS
html {
  scroll-behavior: smooth;
}

// Using Lenis for butter-smooth scrolling
import Lenis from '@studio-freight/lenis';

function SmoothScrollProvider({ children }) {
  useEffect(() => {
    const lenis = new Lenis({
      duration: 1.2,
      easing: (t) => Math.min(1, 1.001 - Math.pow(2, -10 * t)),
      direction: 'vertical',
      smooth: true,
    });

    function raf(time) {
      lenis.raf(time);
      requestAnimationFrame(raf);
    }
    requestAnimationFrame(raf);

    return () => lenis.destroy();
  }, []);

  return children;
}
```

## Scroll Snap

```css
/* Scroll snap container */
.snap-container {
  scroll-snap-type: y mandatory;
  overflow-y: scroll;
  height: 100vh;
}

.snap-section {
  scroll-snap-align: start;
  height: 100vh;
}

/* Smooth scrolling with snap */
@supports (scroll-snap-type: y mandatory) {
  .snap-container {
    scroll-behavior: smooth;
  }
}
```

```tsx
function FullPageScroll({ sections }) {
  return (
    <div className="snap-container">
      {sections.map((section, i) => (
        <section
          key={i}
          className="snap-section flex items-center justify-center"
        >
          {section}
        </section>
      ))}
    </div>
  );
}
```

## Performance Optimization

```tsx
// Use will-change sparingly
const AnimatedElement = styled(motion.div)`
  will-change: transform;
`;

// Debounce scroll handlers
function useThrottledScroll(callback, delay = 16) {
  const lastRun = useRef(0);

  useEffect(() => {
    const handler = () => {
      const now = Date.now();
      if (now - lastRun.current >= delay) {
        lastRun.current = now;
        callback();
      }
    };

    window.addEventListener("scroll", handler, { passive: true });
    return () => window.removeEventListener("scroll", handler);
  }, [callback, delay]);
}

// Use transform instead of top/left
// Good
const goodAnimation = { transform: "translateY(100px)" };
// Bad (causes reflow)
const badAnimation = { top: "100px" };
```
