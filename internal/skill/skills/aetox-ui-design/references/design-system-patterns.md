
# Design System Patterns

Master design system architecture to create consistent, maintainable, and scalable UI foundations across web and mobile applications.

## When to Use This Skill

- Creating design tokens for colors, typography, spacing, and shadows
- Implementing light/dark theme switching with CSS custom properties
- Building multi-brand theming systems
- Architecting component libraries with consistent APIs
- Establishing design-to-code workflows with Figma tokens
- Creating semantic token hierarchies (primitive, semantic, component)
- Setting up design system documentation and guidelines

## Core Capabilities

### 1. Design Tokens

- Primitive tokens (raw values: colors, sizes, fonts)
- Semantic tokens (contextual meaning: text-primary, surface-elevated)
- Component tokens (specific usage: button-bg, card-border)
- Token naming conventions and organization
- Multi-platform token generation (CSS, iOS, Android)

### 2. Theming Infrastructure

- CSS custom properties architecture
- Theme context providers in React
- Dynamic theme switching
- System preference detection (prefers-color-scheme)
- Persistent theme storage
- Reduced motion and high contrast modes

### 3. Component Architecture

- Compound component patterns
- Polymorphic components (as prop)
- Variant and size systems
- Slot-based composition
- Headless UI patterns
- Style props and responsive variants

### 4. Token Pipeline

- Figma to code synchronization
- Style Dictionary configuration
- Token transformation and formatting
- CI/CD integration for token updates

## Quick Start

```typescript
// Design tokens with CSS custom properties
const tokens = {
  colors: {
    // Primitive tokens
    gray: {
      50: "#fafafa",
      100: "#f5f5f5",
      900: "#171717",
    },
    blue: {
      500: "#3b82f6",
      600: "#2563eb",
    },
  },
  // Semantic tokens (reference primitives)
  semantic: {
    light: {
      "text-primary": "var(--color-gray-900)",
      "text-secondary": "var(--color-gray-600)",
      "surface-default": "var(--color-white)",
      "surface-elevated": "var(--color-gray-50)",
      "border-default": "var(--color-gray-200)",
      "interactive-primary": "var(--color-blue-500)",
    },
    dark: {
      "text-primary": "var(--color-gray-50)",
      "text-secondary": "var(--color-gray-400)",
      "surface-default": "var(--color-gray-900)",
      "surface-elevated": "var(--color-gray-800)",
      "border-default": "var(--color-gray-700)",
      "interactive-primary": "var(--color-blue-400)",
    },
  },
};
```

## Detailed patterns and worked examples

The detailed patterns follow below in this file.

## Best Practices

1. **Name Tokens by Purpose**: Use semantic names (text-primary) not visual descriptions (dark-gray)
2. **Maintain Token Hierarchy**: Primitives > Semantic > Component tokens
3. **Document Token Usage**: Include usage guidelines with token definitions
4. **Version Tokens**: Treat token changes as API changes with semver
5. **Test Theme Combinations**: Verify all themes work with all components
6. **Automate Token Pipeline**: CI/CD for Figma-to-code synchronization
7. **Provide Migration Paths**: Deprecate tokens gradually with clear alternatives

## Common Issues

- **Token Sprawl**: Too many tokens without clear hierarchy
- **Inconsistent Naming**: Mixed conventions (camelCase vs kebab-case)
- **Missing Dark Mode**: Tokens that don't adapt to theme changes
- **Hardcoded Values**: Using raw values instead of tokens
- **Circular References**: Tokens referencing each other in loops
- **Platform Gaps**: Tokens missing for some platforms (web but not mobile)


---

## อ้างอิง: component-architecture

# Component Architecture Patterns

## Overview

Well-architected components are reusable, composable, and maintainable. This guide covers patterns for building flexible component APIs that scale across design systems.

## Compound Components

Compound components share implicit state through React context, allowing flexible composition.

```tsx
// Compound component pattern
import * as React from "react";

interface AccordionContextValue {
  openItems: Set<string>;
  toggle: (id: string) => void;
  type: "single" | "multiple";
}

const AccordionContext = React.createContext<AccordionContextValue | null>(
  null,
);

function useAccordionContext() {
  const context = React.useContext(AccordionContext);
  if (!context) {
    throw new Error("Accordion components must be used within an Accordion");
  }
  return context;
}

// Root component
interface AccordionProps {
  children: React.ReactNode;
  type?: "single" | "multiple";
  defaultOpen?: string[];
}

function Accordion({
  children,
  type = "single",
  defaultOpen = [],
}: AccordionProps) {
  const [openItems, setOpenItems] = React.useState<Set<string>>(
    new Set(defaultOpen),
  );

  const toggle = React.useCallback(
    (id: string) => {
      setOpenItems((prev) => {
        const next = new Set(prev);
        if (next.has(id)) {
          next.delete(id);
        } else {
          if (type === "single") {
            next.clear();
          }
          next.add(id);
        }
        return next;
      });
    },
    [type],
  );

  return (
    <AccordionContext.Provider value={{ openItems, toggle, type }}>
      <div className="divide-y divide-border">{children}</div>
    </AccordionContext.Provider>
  );
}

// Item component
interface AccordionItemProps {
  children: React.ReactNode;
  id: string;
}

function AccordionItem({ children, id }: AccordionItemProps) {
  return (
    <AccordionItemContext.Provider value={{ id }}>
      <div className="py-2">{children}</div>
    </AccordionItemContext.Provider>
  );
}

// Trigger component
function AccordionTrigger({ children }: { children: React.ReactNode }) {
  const { toggle, openItems } = useAccordionContext();
  const { id } = useAccordionItemContext();
  const isOpen = openItems.has(id);

  return (
    <button
      onClick={() => toggle(id)}
      className="flex w-full items-center justify-between py-2 font-medium"
      aria-expanded={isOpen}
    >
      {children}
      <ChevronDown
        className={`h-4 w-4 transition-transform ${isOpen ? "rotate-180" : ""}`}
      />
    </button>
  );
}

// Content component
function AccordionContent({ children }: { children: React.ReactNode }) {
  const { openItems } = useAccordionContext();
  const { id } = useAccordionItemContext();
  const isOpen = openItems.has(id);

  if (!isOpen) return null;

  return <div className="pb-4 text-muted-foreground">{children}</div>;
}

// Export compound component
export const AccordionCompound = Object.assign(Accordion, {
  Item: AccordionItem,
  Trigger: AccordionTrigger,
  Content: AccordionContent,
});

// Usage
function Example() {
  return (
    <AccordionCompound type="single" defaultOpen={["item-1"]}>
      <AccordionCompound.Item id="item-1">
        <AccordionCompound.Trigger>Is it accessible?</AccordionCompound.Trigger>
        <AccordionCompound.Content>
          Yes. It follows WAI-ARIA patterns.
        </AccordionCompound.Content>
      </AccordionCompound.Item>
      <AccordionCompound.Item id="item-2">
        <AccordionCompound.Trigger>Is it styled?</AccordionCompound.Trigger>
        <AccordionCompound.Content>
          Yes. It uses Tailwind CSS.
        </AccordionCompound.Content>
      </AccordionCompound.Item>
    </AccordionCompound>
  );
}
```

## Polymorphic Components

Polymorphic components can render as different HTML elements or other components.

```tsx
// Polymorphic component with proper TypeScript support
import * as React from "react";

type AsProp<C extends React.ElementType> = {
  as?: C;
};

type PropsToOmit<C extends React.ElementType, P> = keyof (AsProp<C> & P);

type PolymorphicComponentProp<
  C extends React.ElementType,
  Props = {},
> = React.PropsWithChildren<Props & AsProp<C>> &
  Omit<React.ComponentPropsWithoutRef<C>, PropsToOmit<C, Props>>;

type PolymorphicRef<C extends React.ElementType> =
  React.ComponentPropsWithRef<C>["ref"];

type PolymorphicComponentPropWithRef<
  C extends React.ElementType,
  Props = {},
> = PolymorphicComponentProp<C, Props> & { ref?: PolymorphicRef<C> };

// Button component
interface ButtonOwnProps {
  variant?: "default" | "outline" | "ghost";
  size?: "sm" | "md" | "lg";
}

type ButtonProps<C extends React.ElementType = "button"> =
  PolymorphicComponentPropWithRef<C, ButtonOwnProps>;

const Button = React.forwardRef(
  <C extends React.ElementType = "button">(
    {
      as,
      variant = "default",
      size = "md",
      className,
      children,
      ...props
    }: ButtonProps<C>,
    ref?: PolymorphicRef<C>,
  ) => {
    const Component = as || "button";

    const variantClasses = {
      default: "bg-primary text-primary-foreground hover:bg-primary/90",
      outline: "border border-input bg-background hover:bg-accent",
      ghost: "hover:bg-accent hover:text-accent-foreground",
    };

    const sizeClasses = {
      sm: "h-8 px-3 text-sm",
      md: "h-10 px-4 text-sm",
      lg: "h-12 px-6 text-base",
    };

    return (
      <Component
        ref={ref}
        className={cn(
          "inline-flex items-center justify-center rounded-md font-medium transition-colors",
          variantClasses[variant],
          sizeClasses[size],
          className,
        )}
        {...props}
      >
        {children}
      </Component>
    );
  },
);

Button.displayName = "Button";

// Usage
function Example() {
  return (
    <>
      {/* As button (default) */}
      <Button variant="default" onClick={() => {}}>
        Click me
      </Button>

      {/* As anchor link */}
      <Button as="a" href="/page" variant="outline">
        Go to page
      </Button>

      {/* As Next.js Link */}
      <Button as={Link} href="/dashboard" variant="ghost">
        Dashboard
      </Button>
    </>
  );
}
```

## Slot Pattern

Slots allow users to replace default elements with custom implementations.

```tsx
// Slot pattern for customizable components
import * as React from "react";
import { Slot } from "@radix-ui/react-slot";

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  asChild?: boolean;
  variant?: "default" | "outline";
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ asChild = false, variant = "default", className, ...props }, ref) => {
    const Comp = asChild ? Slot : "button";

    return (
      <Comp
        ref={ref}
        className={cn(
          "inline-flex items-center justify-center rounded-md font-medium",
          variant === "default" && "bg-primary text-primary-foreground",
          variant === "outline" && "border border-input bg-background",
          className,
        )}
        {...props}
      />
    );
  },
);

// Usage - Button styles applied to child element
function Example() {
  return (
    <Button asChild variant="outline">
      <a href="/link">I'm a link that looks like a button</a>
    </Button>
  );
}
```

## Headless Components

Headless components provide behavior without styling, enabling complete visual customization.

```tsx
// Headless toggle hook
import * as React from "react";

interface UseToggleProps {
  defaultPressed?: boolean;
  pressed?: boolean;
  onPressedChange?: (pressed: boolean) => void;
}

function useToggle({
  defaultPressed = false,
  pressed: controlledPressed,
  onPressedChange,
}: UseToggleProps = {}) {
  const [uncontrolledPressed, setUncontrolledPressed] =
    React.useState(defaultPressed);

  const isControlled = controlledPressed !== undefined;
  const pressed = isControlled ? controlledPressed : uncontrolledPressed;

  const toggle = React.useCallback(() => {
    if (!isControlled) {
      setUncontrolledPressed((prev) => !prev);
    }
    onPressedChange?.(!pressed);
  }, [isControlled, pressed, onPressedChange]);

  return {
    pressed,
    toggle,
    buttonProps: {
      role: "switch" as const,
      "aria-checked": pressed,
      onClick: toggle,
    },
  };
}

// Headless listbox hook
interface UseListboxProps<T> {
  items: T[];
  defaultSelectedIndex?: number;
  onSelect?: (item: T, index: number) => void;
}

function useListbox<T>({
  items,
  defaultSelectedIndex = -1,
  onSelect,
}: UseListboxProps<T>) {
  const [selectedIndex, setSelectedIndex] =
    React.useState(defaultSelectedIndex);
  const [highlightedIndex, setHighlightedIndex] = React.useState(-1);

  const select = React.useCallback(
    (index: number) => {
      setSelectedIndex(index);
      onSelect?.(items[index], index);
    },
    [items, onSelect],
  );

  const handleKeyDown = React.useCallback(
    (event: React.KeyboardEvent) => {
      switch (event.key) {
        case "ArrowDown":
          event.preventDefault();
          setHighlightedIndex((prev) =>
            prev < items.length - 1 ? prev + 1 : prev,
          );
          break;
        case "ArrowUp":
          event.preventDefault();
          setHighlightedIndex((prev) => (prev > 0 ? prev - 1 : prev));
          break;
        case "Enter":
        case " ":
          event.preventDefault();
          if (highlightedIndex >= 0) {
            select(highlightedIndex);
          }
          break;
        case "Home":
          event.preventDefault();
          setHighlightedIndex(0);
          break;
        case "End":
          event.preventDefault();
          setHighlightedIndex(items.length - 1);
          break;
      }
    },
    [items.length, highlightedIndex, select],
  );

  return {
    selectedIndex,
    highlightedIndex,
    select,
    setHighlightedIndex,
    listboxProps: {
      role: "listbox" as const,
      tabIndex: 0,
      onKeyDown: handleKeyDown,
    },
    getOptionProps: (index: number) => ({
      role: "option" as const,
      "aria-selected": index === selectedIndex,
      onClick: () => select(index),
      onMouseEnter: () => setHighlightedIndex(index),
    }),
  };
}
```

## Variant System with CVA

Class Variance Authority (CVA) provides type-safe variant management.

```tsx
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@/lib/utils';

// Define variants
const badgeVariants = cva(
  // Base classes
  'inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold transition-colors',
  {
    variants: {
      variant: {
        default: 'border-transparent bg-primary text-primary-foreground',
        secondary: 'border-transparent bg-secondary text-secondary-foreground',
        destructive: 'border-transparent bg-destructive text-destructive-foreground',
        outline: 'text-foreground',
        success: 'border-transparent bg-green-500 text-white',
        warning: 'border-transparent bg-amber-500 text-white',
      },
      size: {
        sm: 'text-xs px-2 py-0.5',
        md: 'text-sm px-2.5 py-0.5',
        lg: 'text-sm px-3 py-1',
      },
    },
    compoundVariants: [
      // Outline variant with sizes
      {
        variant: 'outline',
        size: 'lg',
        className: 'border-2',
      },
    ],
    defaultVariants: {
      variant: 'default',
      size: 'md',
    },
  }
);

// Component with variants
interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, size, ...props }: BadgeProps) {
  return (
    <div className={cn(badgeVariants({ variant, size, className }))} {...props} />
  );
}

// Usage
<Badge variant="success" size="lg">Active</Badge>
<Badge variant="destructive">Error</Badge>
<Badge variant="outline">Draft</Badge>
```

## Responsive Variants

```tsx
import { cva } from "class-variance-authority";

// Responsive variant configuration
const containerVariants = cva("mx-auto w-full px-4", {
  variants: {
    size: {
      sm: "max-w-screen-sm",
      md: "max-w-screen-md",
      lg: "max-w-screen-lg",
      xl: "max-w-screen-xl",
      full: "max-w-full",
    },
    padding: {
      none: "px-0",
      sm: "px-4 md:px-6",
      md: "px-4 md:px-8 lg:px-12",
      lg: "px-6 md:px-12 lg:px-20",
    },
  },
  defaultVariants: {
    size: "lg",
    padding: "md",
  },
});

// Responsive prop pattern
interface ResponsiveValue<T> {
  base?: T;
  sm?: T;
  md?: T;
  lg?: T;
  xl?: T;
}

function getResponsiveClasses<T extends string>(
  prop: T | ResponsiveValue<T> | undefined,
  classMap: Record<T, string>,
  responsiveClassMap: Record<string, Record<T, string>>,
): string {
  if (!prop) return "";

  if (typeof prop === "string") {
    return classMap[prop];
  }

  return Object.entries(prop)
    .map(([breakpoint, value]) => {
      if (breakpoint === "base") {
        return classMap[value as T];
      }
      return responsiveClassMap[breakpoint]?.[value as T];
    })
    .filter(Boolean)
    .join(" ");
}
```

## Composition Patterns

### Render Props

```tsx
interface DataListProps<T> {
  items: T[];
  renderItem: (item: T, index: number) => React.ReactNode;
  renderEmpty?: () => React.ReactNode;
  keyExtractor: (item: T) => string;
}

function DataList<T>({
  items,
  renderItem,
  renderEmpty,
  keyExtractor,
}: DataListProps<T>) {
  if (items.length === 0 && renderEmpty) {
    return <>{renderEmpty()}</>;
  }

  return (
    <ul className="space-y-2">
      {items.map((item, index) => (
        <li key={keyExtractor(item)}>{renderItem(item, index)}</li>
      ))}
    </ul>
  );
}

// Usage
<DataList
  items={users}
  keyExtractor={(user) => user.id}
  renderItem={(user) => <UserCard user={user} />}
  renderEmpty={() => <EmptyState message="No users found" />}
/>;
```

### Children as Function

```tsx
interface DisclosureProps {
  children: (props: { isOpen: boolean; toggle: () => void }) => React.ReactNode;
  defaultOpen?: boolean;
}

function Disclosure({ children, defaultOpen = false }: DisclosureProps) {
  const [isOpen, setIsOpen] = React.useState(defaultOpen);
  const toggle = () => setIsOpen((prev) => !prev);

  return <>{children({ isOpen, toggle })}</>;
}

// Usage
<Disclosure>
  {({ isOpen, toggle }) => (
    <>
      <button onClick={toggle}>{isOpen ? "Close" : "Open"}</button>
      {isOpen && <div>Content</div>}
    </>
  )}
</Disclosure>;
```

## Best Practices

1. **Prefer Composition**: Build complex components from simple primitives
2. **Use Controlled/Uncontrolled Pattern**: Support both modes for flexibility
3. **Forward Refs**: Always forward refs to root elements
4. **Spread Props**: Allow custom props to pass through
5. **Provide Defaults**: Set sensible defaults for optional props
6. **Type Everything**: Use TypeScript for prop validation
7. **Document Variants**: Show all variant combinations in Storybook
8. **Test Accessibility**: Verify keyboard navigation and screen reader support

## Resources

- [Radix UI Primitives](https://www.radix-ui.com/primitives)
- [Headless UI](https://headlessui.com/)
- [Class Variance Authority](https://cva.style/docs)
- [React Aria](https://react-spectrum.adobe.com/react-aria/)


---

## อ้างอิง: design-tokens

# Design Tokens Deep Dive

## Overview

Design tokens are the atomic values of a design system - the smallest pieces that define visual style. They bridge the gap between design and development by providing a single source of truth for colors, typography, spacing, and other design decisions.

## Token Categories

### Color Tokens

```json
{
  "color": {
    "primitive": {
      "gray": {
        "0": { "value": "#ffffff" },
        "50": { "value": "#fafafa" },
        "100": { "value": "#f5f5f5" },
        "200": { "value": "#e5e5e5" },
        "300": { "value": "#d4d4d4" },
        "400": { "value": "#a3a3a3" },
        "500": { "value": "#737373" },
        "600": { "value": "#525252" },
        "700": { "value": "#404040" },
        "800": { "value": "#262626" },
        "900": { "value": "#171717" },
        "950": { "value": "#0a0a0a" }
      },
      "blue": {
        "50": { "value": "#eff6ff" },
        "100": { "value": "#dbeafe" },
        "200": { "value": "#bfdbfe" },
        "300": { "value": "#93c5fd" },
        "400": { "value": "#60a5fa" },
        "500": { "value": "#3b82f6" },
        "600": { "value": "#2563eb" },
        "700": { "value": "#1d4ed8" },
        "800": { "value": "#1e40af" },
        "900": { "value": "#1e3a8a" }
      },
      "red": {
        "500": { "value": "#ef4444" },
        "600": { "value": "#dc2626" }
      },
      "green": {
        "500": { "value": "#22c55e" },
        "600": { "value": "#16a34a" }
      },
      "amber": {
        "500": { "value": "#f59e0b" },
        "600": { "value": "#d97706" }
      }
    }
  }
}
```

### Typography Tokens

```json
{
  "typography": {
    "fontFamily": {
      "sans": { "value": "Inter, system-ui, sans-serif" },
      "mono": { "value": "JetBrains Mono, Menlo, monospace" }
    },
    "fontSize": {
      "xs": { "value": "0.75rem" },
      "sm": { "value": "0.875rem" },
      "base": { "value": "1rem" },
      "lg": { "value": "1.125rem" },
      "xl": { "value": "1.25rem" },
      "2xl": { "value": "1.5rem" },
      "3xl": { "value": "1.875rem" },
      "4xl": { "value": "2.25rem" }
    },
    "fontWeight": {
      "normal": { "value": "400" },
      "medium": { "value": "500" },
      "semibold": { "value": "600" },
      "bold": { "value": "700" }
    },
    "lineHeight": {
      "tight": { "value": "1.25" },
      "normal": { "value": "1.5" },
      "relaxed": { "value": "1.75" }
    },
    "letterSpacing": {
      "tight": { "value": "-0.025em" },
      "normal": { "value": "0" },
      "wide": { "value": "0.025em" }
    }
  }
}
```

### Spacing Tokens

```json
{
  "spacing": {
    "0": { "value": "0" },
    "0.5": { "value": "0.125rem" },
    "1": { "value": "0.25rem" },
    "1.5": { "value": "0.375rem" },
    "2": { "value": "0.5rem" },
    "2.5": { "value": "0.625rem" },
    "3": { "value": "0.75rem" },
    "3.5": { "value": "0.875rem" },
    "4": { "value": "1rem" },
    "5": { "value": "1.25rem" },
    "6": { "value": "1.5rem" },
    "7": { "value": "1.75rem" },
    "8": { "value": "2rem" },
    "9": { "value": "2.25rem" },
    "10": { "value": "2.5rem" },
    "12": { "value": "3rem" },
    "14": { "value": "3.5rem" },
    "16": { "value": "4rem" },
    "20": { "value": "5rem" },
    "24": { "value": "6rem" }
  }
}
```

### Effects Tokens

```json
{
  "shadow": {
    "sm": { "value": "0 1px 2px 0 rgb(0 0 0 / 0.05)" },
    "md": {
      "value": "0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1)"
    },
    "lg": {
      "value": "0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1)"
    },
    "xl": {
      "value": "0 20px 25px -5px rgb(0 0 0 / 0.1), 0 8px 10px -6px rgb(0 0 0 / 0.1)"
    }
  },
  "radius": {
    "none": { "value": "0" },
    "sm": { "value": "0.125rem" },
    "md": { "value": "0.375rem" },
    "lg": { "value": "0.5rem" },
    "xl": { "value": "0.75rem" },
    "2xl": { "value": "1rem" },
    "full": { "value": "9999px" }
  },
  "opacity": {
    "0": { "value": "0" },
    "25": { "value": "0.25" },
    "50": { "value": "0.5" },
    "75": { "value": "0.75" },
    "100": { "value": "1" }
  }
}
```

## Semantic Token Mapping

### Light Theme

```json
{
  "semantic": {
    "light": {
      "background": {
        "default": { "value": "{color.primitive.gray.0}" },
        "subtle": { "value": "{color.primitive.gray.50}" },
        "muted": { "value": "{color.primitive.gray.100}" },
        "emphasis": { "value": "{color.primitive.gray.900}" }
      },
      "foreground": {
        "default": { "value": "{color.primitive.gray.900}" },
        "muted": { "value": "{color.primitive.gray.600}" },
        "subtle": { "value": "{color.primitive.gray.400}" },
        "onEmphasis": { "value": "{color.primitive.gray.0}" }
      },
      "border": {
        "default": { "value": "{color.primitive.gray.200}" },
        "muted": { "value": "{color.primitive.gray.100}" },
        "emphasis": { "value": "{color.primitive.gray.900}" }
      },
      "accent": {
        "default": { "value": "{color.primitive.blue.500}" },
        "emphasis": { "value": "{color.primitive.blue.600}" },
        "muted": { "value": "{color.primitive.blue.100}" },
        "subtle": { "value": "{color.primitive.blue.50}" }
      },
      "success": {
        "default": { "value": "{color.primitive.green.500}" },
        "emphasis": { "value": "{color.primitive.green.600}" }
      },
      "warning": {
        "default": { "value": "{color.primitive.amber.500}" },
        "emphasis": { "value": "{color.primitive.amber.600}" }
      },
      "danger": {
        "default": { "value": "{color.primitive.red.500}" },
        "emphasis": { "value": "{color.primitive.red.600}" }
      }
    }
  }
}
```

### Dark Theme

```json
{
  "semantic": {
    "dark": {
      "background": {
        "default": { "value": "{color.primitive.gray.950}" },
        "subtle": { "value": "{color.primitive.gray.900}" },
        "muted": { "value": "{color.primitive.gray.800}" },
        "emphasis": { "value": "{color.primitive.gray.50}" }
      },
      "foreground": {
        "default": { "value": "{color.primitive.gray.50}" },
        "muted": { "value": "{color.primitive.gray.400}" },
        "subtle": { "value": "{color.primitive.gray.500}" },
        "onEmphasis": { "value": "{color.primitive.gray.950}" }
      },
      "border": {
        "default": { "value": "{color.primitive.gray.800}" },
        "muted": { "value": "{color.primitive.gray.900}" },
        "emphasis": { "value": "{color.primitive.gray.50}" }
      },
      "accent": {
        "default": { "value": "{color.primitive.blue.400}" },
        "emphasis": { "value": "{color.primitive.blue.300}" },
        "muted": { "value": "{color.primitive.blue.900}" },
        "subtle": { "value": "{color.primitive.blue.950}" }
      }
    }
  }
}
```

## Token Naming Conventions

### Recommended Structure

```
[category]-[property]-[variant]-[state]

Examples:
- color-background-default
- color-text-primary
- color-border-input-focus
- spacing-component-padding
- typography-heading-lg
```

### Naming Guidelines

1. **Use kebab-case**: `text-primary` not `textPrimary`
2. **Be descriptive**: `button-padding-horizontal` not `btn-px`
3. **Use semantic names**: `danger` not `red`
4. **Include scale info**: `spacing-4` or `font-size-lg`
5. **State suffixes**: `-hover`, `-focus`, `-active`, `-disabled`

## CSS Custom Properties Output

```css
:root {
  /* Primitives */
  --color-gray-50: #fafafa;
  --color-gray-100: #f5f5f5;
  --color-gray-900: #171717;
  --color-blue-500: #3b82f6;

  --spacing-1: 0.25rem;
  --spacing-2: 0.5rem;
  --spacing-4: 1rem;

  --radius-md: 0.375rem;
  --radius-lg: 0.5rem;

  --shadow-sm: 0 1px 2px 0 rgb(0 0 0 / 0.05);
  --shadow-md: 0 4px 6px -1px rgb(0 0 0 / 0.1);

  /* Semantic - Light Theme */
  --background-default: var(--color-white);
  --background-subtle: var(--color-gray-50);
  --foreground-default: var(--color-gray-900);
  --foreground-muted: var(--color-gray-600);
  --border-default: var(--color-gray-200);
  --accent-default: var(--color-blue-500);
}

.dark {
  /* Semantic - Dark Theme Overrides */
  --background-default: var(--color-gray-950);
  --background-subtle: var(--color-gray-900);
  --foreground-default: var(--color-gray-50);
  --foreground-muted: var(--color-gray-400);
  --border-default: var(--color-gray-800);
  --accent-default: var(--color-blue-400);
}
```

## Token Transformations

### Style Dictionary Transforms

```javascript
const StyleDictionary = require("style-dictionary");

// Custom transform for px to rem
StyleDictionary.registerTransform({
  name: "size/pxToRem",
  type: "value",
  matcher: (token) => token.attributes.category === "size",
  transformer: (token) => {
    const value = parseFloat(token.value);
    return `${value / 16}rem`;
  },
});

// Custom format for CSS custom properties
StyleDictionary.registerFormat({
  name: "css/customProperties",
  formatter: function ({ dictionary, options }) {
    const tokens = dictionary.allTokens.map((token) => {
      const name = token.name.replace(/\./g, "-");
      return `  --${name}: ${token.value};`;
    });

    return `:root {\n${tokens.join("\n")}\n}`;
  },
});
```

### Platform-Specific Outputs

```javascript
// iOS Swift output
public enum DesignTokens {
    public enum Color {
        public static let gray50 = UIColor(hex: "#fafafa")
        public static let gray900 = UIColor(hex: "#171717")
        public static let blue500 = UIColor(hex: "#3b82f6")
    }

    public enum Spacing {
        public static let space1: CGFloat = 4
        public static let space2: CGFloat = 8
        public static let space4: CGFloat = 16
    }
}

// Android XML output
<resources>
    <color name="gray_50">#fafafa</color>
    <color name="gray_900">#171717</color>
    <color name="blue_500">#3b82f6</color>

    <dimen name="spacing_1">4dp</dimen>
    <dimen name="spacing_2">8dp</dimen>
    <dimen name="spacing_4">16dp</dimen>
</resources>
```

## Token Governance

### Change Management

1. **Propose**: Document the change and rationale
2. **Review**: Design and engineering review
3. **Test**: Validate across all platforms
4. **Communicate**: Announce changes to consumers
5. **Deprecate**: Mark old tokens, provide migration path
6. **Remove**: After deprecation period

### Deprecation Pattern

```json
{
  "color": {
    "primary": {
      "value": "{color.primitive.blue.500}",
      "deprecated": true,
      "deprecatedMessage": "Use accent.default instead",
      "replacedBy": "semantic.accent.default"
    }
  }
}
```

## Token Validation

```typescript
interface TokenValidation {
  checkContrastRatios(): ContrastReport;
  validateReferences(): ReferenceReport;
  detectCircularDeps(): CircularDepReport;
  auditNaming(): NamingReport;
}

// Contrast validation
function validateContrast(
  foreground: string,
  background: string,
  level: "AA" | "AAA" = "AA",
): boolean {
  const ratio = getContrastRatio(foreground, background);
  return level === "AA" ? ratio >= 4.5 : ratio >= 7;
}
```

## Resources

- [Design Tokens W3C Community Group](https://design-tokens.github.io/community-group/)
- [Style Dictionary](https://amzn.github.io/style-dictionary/)
- [Tokens Studio](https://tokens.studio/)
- [Open Props](https://open-props.style/)


---

## อ้างอิง: details

# design-system-patterns, detailed patterns and worked examples

## Key Patterns

### Pattern 1: Token Hierarchy

```css
/* Layer 1: Primitive tokens (raw values) */
:root {
  --color-blue-500: #3b82f6;
  --color-blue-600: #2563eb;
  --color-gray-50: #fafafa;
  --color-gray-900: #171717;

  --space-1: 0.25rem;
  --space-2: 0.5rem;
  --space-4: 1rem;

  --font-size-sm: 0.875rem;
  --font-size-base: 1rem;
  --font-size-lg: 1.125rem;

  --radius-sm: 0.25rem;
  --radius-md: 0.5rem;
  --radius-lg: 1rem;
}

/* Layer 2: Semantic tokens (meaning) */
:root {
  --text-primary: var(--color-gray-900);
  --text-secondary: var(--color-gray-600);
  --surface-default: white;
  --interactive-primary: var(--color-blue-500);
  --interactive-primary-hover: var(--color-blue-600);
}

/* Layer 3: Component tokens (specific usage) */
:root {
  --button-bg: var(--interactive-primary);
  --button-bg-hover: var(--interactive-primary-hover);
  --button-text: white;
  --button-radius: var(--radius-md);
  --button-padding-x: var(--space-4);
  --button-padding-y: var(--space-2);
}
```

### Pattern 2: Theme Switching with React

```tsx
import { createContext, useContext, useEffect, useState } from "react";

type Theme = "light" | "dark" | "system";

interface ThemeContextValue {
  theme: Theme;
  resolvedTheme: "light" | "dark";
  setTheme: (theme: Theme) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [theme, setTheme] = useState<Theme>(() => {
    if (typeof window !== "undefined") {
      return (localStorage.getItem("theme") as Theme) || "system";
    }
    return "system";
  });

  const [resolvedTheme, setResolvedTheme] = useState<"light" | "dark">("light");

  useEffect(() => {
    const root = document.documentElement;

    const applyTheme = (isDark: boolean) => {
      root.classList.remove("light", "dark");
      root.classList.add(isDark ? "dark" : "light");
      setResolvedTheme(isDark ? "dark" : "light");
    };

    if (theme === "system") {
      const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
      applyTheme(mediaQuery.matches);

      const handler = (e: MediaQueryListEvent) => applyTheme(e.matches);
      mediaQuery.addEventListener("change", handler);
      return () => mediaQuery.removeEventListener("change", handler);
    } else {
      applyTheme(theme === "dark");
    }
  }, [theme]);

  useEffect(() => {
    localStorage.setItem("theme", theme);
  }, [theme]);

  return (
    <ThemeContext.Provider value={{ theme, resolvedTheme, setTheme }}>
      {children}
    </ThemeContext.Provider>
  );
}

export const useTheme = () => {
  const context = useContext(ThemeContext);
  if (!context) throw new Error("useTheme must be used within ThemeProvider");
  return context;
};
```

### Pattern 3: Variant System with CVA

```tsx
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  // Base styles
  "inline-flex items-center justify-center rounded-md font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground hover:bg-primary/90",
        destructive:
          "bg-destructive text-destructive-foreground hover:bg-destructive/90",
        outline:
          "border border-input bg-background hover:bg-accent hover:text-accent-foreground",
        secondary:
          "bg-secondary text-secondary-foreground hover:bg-secondary/80",
        ghost: "hover:bg-accent hover:text-accent-foreground",
        link: "text-primary underline-offset-4 hover:underline",
      },
      size: {
        sm: "h-9 px-3 text-sm",
        md: "h-10 px-4 text-sm",
        lg: "h-11 px-8 text-base",
        icon: "h-10 w-10",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "md",
    },
  },
);

interface ButtonProps
  extends
    React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

export function Button({ className, variant, size, ...props }: ButtonProps) {
  return (
    <button
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  );
}
```

### Pattern 4: Style Dictionary Configuration

```javascript
// style-dictionary.config.js
module.exports = {
  source: ["tokens/**/*.json"],
  platforms: {
    css: {
      transformGroup: "css",
      buildPath: "dist/css/",
      files: [
        {
          destination: "variables.css",
          format: "css/variables",
          options: {
            outputReferences: true, // Preserve token references
          },
        },
      ],
    },
    scss: {
      transformGroup: "scss",
      buildPath: "dist/scss/",
      files: [
        {
          destination: "_variables.scss",
          format: "scss/variables",
        },
      ],
    },
    ios: {
      transformGroup: "ios-swift",
      buildPath: "dist/ios/",
      files: [
        {
          destination: "DesignTokens.swift",
          format: "ios-swift/class.swift",
          className: "DesignTokens",
        },
      ],
    },
    android: {
      transformGroup: "android",
      buildPath: "dist/android/",
      files: [
        {
          destination: "colors.xml",
          format: "android/colors",
          filter: { attributes: { category: "color" } },
        },
      ],
    },
  },
};
```


---

## อ้างอิง: theming-architecture

# Theming Architecture

## Overview

A robust theming system enables applications to support multiple visual appearances (light/dark modes, brand themes) while maintaining consistency and developer experience.

## CSS Custom Properties Architecture

### Base Setup

```css
/* 1. Define the token contract */
:root {
  /* Color scheme */
  color-scheme: light dark;

  /* Base tokens that don't change */
  --font-sans: Inter, system-ui, sans-serif;
  --font-mono: "JetBrains Mono", monospace;

  /* Animation tokens */
  --duration-fast: 150ms;
  --duration-normal: 250ms;
  --duration-slow: 400ms;
  --ease-default: cubic-bezier(0.4, 0, 0.2, 1);

  /* Z-index scale */
  --z-dropdown: 100;
  --z-sticky: 200;
  --z-modal: 300;
  --z-popover: 400;
  --z-tooltip: 500;
}

/* 2. Light theme (default) */
:root,
[data-theme="light"] {
  --color-bg: #ffffff;
  --color-bg-subtle: #f8fafc;
  --color-bg-muted: #f1f5f9;
  --color-bg-emphasis: #0f172a;

  --color-text: #0f172a;
  --color-text-muted: #475569;
  --color-text-subtle: #94a3b8;

  --color-border: #e2e8f0;
  --color-border-muted: #f1f5f9;

  --color-accent: #3b82f6;
  --color-accent-hover: #2563eb;
  --color-accent-muted: #dbeafe;

  --shadow-sm: 0 1px 2px 0 rgb(0 0 0 / 0.05);
  --shadow-md: 0 4px 6px -1px rgb(0 0 0 / 0.1);
  --shadow-lg: 0 10px 15px -3px rgb(0 0 0 / 0.1);
}

/* 3. Dark theme */
[data-theme="dark"] {
  --color-bg: #0f172a;
  --color-bg-subtle: #1e293b;
  --color-bg-muted: #334155;
  --color-bg-emphasis: #f8fafc;

  --color-text: #f8fafc;
  --color-text-muted: #94a3b8;
  --color-text-subtle: #64748b;

  --color-border: #334155;
  --color-border-muted: #1e293b;

  --color-accent: #60a5fa;
  --color-accent-hover: #93c5fd;
  --color-accent-muted: #1e3a5f;

  --shadow-sm: 0 1px 2px 0 rgb(0 0 0 / 0.3);
  --shadow-md: 0 4px 6px -1px rgb(0 0 0 / 0.4);
  --shadow-lg: 0 10px 15px -3px rgb(0 0 0 / 0.5);
}

/* 4. System preference detection */
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) {
    /* Inherit dark theme values */
    --color-bg: #0f172a;
    /* ... other dark values */
  }
}
```

### Using Tokens in Components

```css
.card {
  background: var(--color-bg-subtle);
  border: 1px solid var(--color-border);
  border-radius: 0.5rem;
  box-shadow: var(--shadow-sm);
  padding: 1.5rem;
}

.card-title {
  color: var(--color-text);
  font-family: var(--font-sans);
  font-size: 1.125rem;
  font-weight: 600;
}

.card-description {
  color: var(--color-text-muted);
  margin-top: 0.5rem;
}

.button-primary {
  background: var(--color-accent);
  color: white;
  transition: background var(--duration-fast) var(--ease-default);
}

.button-primary:hover {
  background: var(--color-accent-hover);
}
```

## React Theme Provider

### Complete Implementation

```tsx
// theme-provider.tsx
import * as React from "react";

type Theme = "light" | "dark" | "system";
type ResolvedTheme = "light" | "dark";

interface ThemeProviderProps {
  children: React.ReactNode;
  defaultTheme?: Theme;
  storageKey?: string;
  attribute?: "class" | "data-theme";
  enableSystem?: boolean;
  disableTransitionOnChange?: boolean;
}

interface ThemeProviderState {
  theme: Theme;
  resolvedTheme: ResolvedTheme;
  setTheme: (theme: Theme) => void;
  toggleTheme: () => void;
}

const ThemeProviderContext = React.createContext<
  ThemeProviderState | undefined
>(undefined);

export function ThemeProvider({
  children,
  defaultTheme = "system",
  storageKey = "theme",
  attribute = "data-theme",
  enableSystem = true,
  disableTransitionOnChange = false,
}: ThemeProviderProps) {
  const [theme, setThemeState] = React.useState<Theme>(() => {
    if (typeof window === "undefined") return defaultTheme;
    return (localStorage.getItem(storageKey) as Theme) || defaultTheme;
  });

  const [resolvedTheme, setResolvedTheme] =
    React.useState<ResolvedTheme>("light");

  // Get system preference
  const getSystemTheme = React.useCallback((): ResolvedTheme => {
    if (typeof window === "undefined") return "light";
    return window.matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light";
  }, []);

  // Apply theme to DOM
  const applyTheme = React.useCallback(
    (newTheme: ResolvedTheme) => {
      const root = document.documentElement;

      // Disable transitions temporarily
      if (disableTransitionOnChange) {
        const css = document.createElement("style");
        css.appendChild(
          document.createTextNode(
            `*,*::before,*::after{transition:none!important}`,
          ),
        );
        document.head.appendChild(css);

        // Force repaint
        (() => window.getComputedStyle(document.body))();

        // Remove after a tick
        setTimeout(() => {
          document.head.removeChild(css);
        }, 1);
      }

      // Apply attribute
      if (attribute === "class") {
        root.classList.remove("light", "dark");
        root.classList.add(newTheme);
      } else {
        root.setAttribute(attribute, newTheme);
      }

      // Update color-scheme for native elements
      root.style.colorScheme = newTheme;

      setResolvedTheme(newTheme);
    },
    [attribute, disableTransitionOnChange],
  );

  // Handle theme changes
  React.useEffect(() => {
    const resolved = theme === "system" ? getSystemTheme() : theme;
    applyTheme(resolved);
  }, [theme, applyTheme, getSystemTheme]);

  // Listen for system theme changes
  React.useEffect(() => {
    if (!enableSystem || theme !== "system") return;

    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");

    const handleChange = () => {
      applyTheme(getSystemTheme());
    };

    mediaQuery.addEventListener("change", handleChange);
    return () => mediaQuery.removeEventListener("change", handleChange);
  }, [theme, enableSystem, applyTheme, getSystemTheme]);

  // Persist to localStorage
  const setTheme = React.useCallback(
    (newTheme: Theme) => {
      localStorage.setItem(storageKey, newTheme);
      setThemeState(newTheme);
    },
    [storageKey],
  );

  const toggleTheme = React.useCallback(() => {
    setTheme(resolvedTheme === "light" ? "dark" : "light");
  }, [resolvedTheme, setTheme]);

  const value = React.useMemo(
    () => ({
      theme,
      resolvedTheme,
      setTheme,
      toggleTheme,
    }),
    [theme, resolvedTheme, setTheme, toggleTheme],
  );

  return (
    <ThemeProviderContext.Provider value={value}>
      {children}
    </ThemeProviderContext.Provider>
  );
}

export function useTheme() {
  const context = React.useContext(ThemeProviderContext);
  if (context === undefined) {
    throw new Error("useTheme must be used within a ThemeProvider");
  }
  return context;
}
```

### Theme Toggle Component

```tsx
// theme-toggle.tsx
import { Moon, Sun, Monitor } from "lucide-react";
import { useTheme } from "./theme-provider";

export function ThemeToggle() {
  const { theme, setTheme } = useTheme();

  return (
    <div className="flex items-center gap-1 rounded-lg bg-muted p-1">
      <button
        onClick={() => setTheme("light")}
        className={`rounded-md p-2 ${
          theme === "light" ? "bg-background shadow-sm" : ""
        }`}
        aria-label="Light theme"
      >
        <Sun className="h-4 w-4" />
      </button>
      <button
        onClick={() => setTheme("dark")}
        className={`rounded-md p-2 ${
          theme === "dark" ? "bg-background shadow-sm" : ""
        }`}
        aria-label="Dark theme"
      >
        <Moon className="h-4 w-4" />
      </button>
      <button
        onClick={() => setTheme("system")}
        className={`rounded-md p-2 ${
          theme === "system" ? "bg-background shadow-sm" : ""
        }`}
        aria-label="System theme"
      >
        <Monitor className="h-4 w-4" />
      </button>
    </div>
  );
}
```

## Multi-Brand Theming

### Brand Token Structure

```css
/* Brand A - Corporate Blue */
[data-brand="corporate"] {
  --brand-primary: #0066cc;
  --brand-primary-hover: #0052a3;
  --brand-secondary: #f0f7ff;
  --brand-accent: #00a3e0;

  --brand-font-heading: "Helvetica Neue", sans-serif;
  --brand-font-body: "Open Sans", sans-serif;

  --brand-radius: 0.25rem;
  --brand-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
}

/* Brand B - Modern Startup */
[data-brand="startup"] {
  --brand-primary: #7c3aed;
  --brand-primary-hover: #6d28d9;
  --brand-secondary: #faf5ff;
  --brand-accent: #f472b6;

  --brand-font-heading: "Poppins", sans-serif;
  --brand-font-body: "Inter", sans-serif;

  --brand-radius: 1rem;
  --brand-shadow: 0 4px 12px rgba(124, 58, 237, 0.15);
}

/* Brand C - Minimal */
[data-brand="minimal"] {
  --brand-primary: #171717;
  --brand-primary-hover: #404040;
  --brand-secondary: #fafafa;
  --brand-accent: #171717;

  --brand-font-heading: "Space Grotesk", sans-serif;
  --brand-font-body: "IBM Plex Sans", sans-serif;

  --brand-radius: 0;
  --brand-shadow: none;
}
```

## Accessibility Considerations

### Reduced Motion

```css
@media (prefers-reduced-motion: reduce) {
  :root {
    --duration-fast: 0ms;
    --duration-normal: 0ms;
    --duration-slow: 0ms;
  }

  *,
  *::before,
  *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
  }
}
```

### High Contrast Mode

```css
@media (prefers-contrast: high) {
  :root {
    --color-text: #000000;
    --color-text-muted: #000000;
    --color-bg: #ffffff;
    --color-border: #000000;
    --color-accent: #0000ee;
  }

  [data-theme="dark"] {
    --color-text: #ffffff;
    --color-text-muted: #ffffff;
    --color-bg: #000000;
    --color-border: #ffffff;
    --color-accent: #ffff00;
  }
}
```

### Forced Colors

```css
@media (forced-colors: active) {
  .button {
    border: 2px solid currentColor;
  }

  .card {
    border: 1px solid CanvasText;
  }

  .link {
    text-decoration: underline;
  }
}
```

## Server-Side Rendering

### Preventing Flash of Unstyled Content

```tsx
// Inline script to prevent FOUC
const themeScript = `
  (function() {
    const theme = localStorage.getItem('theme') || 'system';
    const isDark = theme === 'dark' ||
      (theme === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);

    document.documentElement.setAttribute('data-theme', isDark ? 'dark' : 'light');
    document.documentElement.style.colorScheme = isDark ? 'dark' : 'light';
  })();
`;

// In Next.js layout - note: inline scripts should be properly sanitized in production
export default function RootLayout({ children }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <script
          // Only use for trusted, static content
          // For dynamic content, use a sanitization library
          dangerouslySetInnerHTML={{ __html: themeScript }}
        />
      </head>
      <body>
        <ThemeProvider>{children}</ThemeProvider>
      </body>
    </html>
  );
}
```

## Testing Themes

```tsx
// theme.test.tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider, useTheme } from "./theme-provider";

function TestComponent() {
  const { theme, setTheme, resolvedTheme } = useTheme();
  return (
    <div>
      <span data-testid="theme">{theme}</span>
      <span data-testid="resolved">{resolvedTheme}</span>
      <button onClick={() => setTheme("dark")}>Set Dark</button>
    </div>
  );
}

describe("ThemeProvider", () => {
  it("should default to system theme", () => {
    render(
      <ThemeProvider>
        <TestComponent />
      </ThemeProvider>,
    );

    expect(screen.getByTestId("theme")).toHaveTextContent("system");
  });

  it("should switch to dark theme", async () => {
    const user = userEvent.setup();

    render(
      <ThemeProvider>
        <TestComponent />
      </ThemeProvider>,
    );

    await user.click(screen.getByText("Set Dark"));
    expect(screen.getByTestId("theme")).toHaveTextContent("dark");
    expect(document.documentElement).toHaveAttribute("data-theme", "dark");
  });
});
```

## Resources

- [Web.dev: prefers-color-scheme](https://web.dev/prefers-color-scheme/)
- [CSS Color Scheme](https://developer.mozilla.org/en-US/docs/Web/CSS/color-scheme)
- [next-themes](https://github.com/pacocoursey/next-themes)
- [Radix UI Colors](https://www.radix-ui.com/colors)
